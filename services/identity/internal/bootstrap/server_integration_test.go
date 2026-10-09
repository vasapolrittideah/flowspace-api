//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	_ "github.com/jackc/pgx/v5/stdlib"

	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
)

func TestIdentityAPIAndWorkerStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	database, err := postgrescontainer.Run(ctx, "postgres:18-alpine",
		postgrescontainer.WithDatabase("identity"), postgrescontainer.WithUsername("identity"),
		postgrescontainer.WithPassword("identity"), postgrescontainer.BasicWaitStrategies())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(database) })
	dsn, err := database.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	keys := t.TempDir()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signingFile := filepath.Join(keys, "signing.pem")
	verifierFile := filepath.Join(keys, "verifier")
	deliveryFile := filepath.Join(keys, "delivery")
	for path, value := range map[string][]byte{
		signingFile:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}),
		verifierFile: bytes.Repeat([]byte{1}, 32), deliveryFile: bytes.Repeat([]byte{2}, 32),
	} {
		if err := os.WriteFile(path, value, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := APIConfig{
		Environment: "local", HTTPAddress: "127.0.0.1:0", InternalHTTPAddress: "127.0.0.1:0",
		DatabaseURL: sharedconfig.Secret(dsn), SigningKeyFile: signingFile, SigningKeyID: "local-1",
		TokenIssuer: "urn:flowspace:identity:local", TokenAudience: "flowspace-api",
		CodeVerifierKeyFile: verifierFile, DeliveryKeyFile: deliveryFile, OutboxReadyMaxPending: 10000,
	}
	if _, err := NewAPIServer(ctx, config, zap.NewNop()); err == nil {
		t.Fatal("API started before migration")
	}
	if err := applyIdentityMigration(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	server, err := NewAPIServer(ctx, config, logger)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.internal.Handler.ServeHTTP(response, httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("API readiness = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.internal.Handler.ServeHTTP(response, httptest.NewRequestWithContext(ctx, http.MethodGet, "/.well-known/jwks.json", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"kid":"local-1"`)) {
		t.Fatalf("JWKS response = %d, %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/unverified-account-claim-codes", bytes.NewBufferString(`{"email":"absent@example.com"}`))
	request.Header.Set("Content-Type", "application/json")
	server.public.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"accepted":true`)) {
		t.Fatalf("claim-code response = %d, %s", response.Code, response.Body.String())
	}
	runCtx, stop := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { result <- server.Run(runCtx) }()
	waitForLog(t, logs, "process_listening")
	stop()
	if err := <-result; err != nil {
		t.Fatalf("API shutdown: %v", err)
	}
	response = httptest.NewRecorder()
	server.internal.Handler.ServeHTTP(response, httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("API readiness after database close = %d", response.Code)
	}
	badListener := config
	badListener.HTTPAddress = "127.0.0.1:-1"
	failedServer, err := NewAPIServer(ctx, badListener, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := failedServer.Run(ctx); err == nil {
		t.Fatal("API accepted invalid listen address")
	}

	broker, err := redpanda.Run(ctx, "docker.redpanda.com/redpandadata/redpanda:v25.2.4",
		redpanda.WithEnableSASL(), redpanda.WithNewServiceAccount("identity-worker", "test"),
		redpanda.WithSuperusers("identity-worker"), redpanda.WithEnableSchemaRegistryHTTPBasicAuth())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(broker) })
	seed, err := broker.KafkaSeedBroker(ctx)
	if err != nil {
		t.Fatal(err)
	}
	registryURL, err := broker.SchemaRegistryAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client, err := kgo.NewClient(kgo.SeedBrokers(seed), kgo.SASL(scram.Auth{User: "identity-worker", Pass: "test"}.AsSha256Mechanism()))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	const topic = "identity-runtime-test"
	if _, err := kadm.NewClient(client).CreateTopic(ctx, 1, 1, nil, topic); err != nil {
		t.Fatal(err)
	}
	workerConfig := WorkerConfig{
		DatabaseURL: sharedconfig.Secret(dsn), DeliveryKeyFile: deliveryFile,
		BrokerAddress: seed, BrokerUsername: "identity-worker", BrokerPassword: "test",
		SchemaRegistryURL: registryURL, DeliveryTopic: topic, DeliveryGroup: "runtime-test",
		MailAddress: "127.0.0.1:1", MailFrom: "codes@example.com", HealthAddress: "127.0.0.1:0",
	}
	badRegistry := workerConfig
	badRegistry.SchemaRegistryURL = "http://127.0.0.1:1"
	if _, err := NewWorker(ctx, badRegistry, logger); err == nil {
		t.Fatal("worker accepted unavailable schema registry")
	}
	worker, err := NewWorker(ctx, workerConfig, logger)
	if err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	worker.health.Handler.ServeHTTP(response, httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("worker readiness = %d", response.Code)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, `INSERT INTO identity_provider_login_attempts
		(provider, attempt_token_verifier, state_verifier, code_verifier, callback_url, created_at, expires_at)
		VALUES ('github', $1, $2, repeat('v', 43), 'http://localhost:8082/v1/provider-login-callbacks/github',
			statement_timestamp() - INTERVAL '11 minutes', statement_timestamp() - INTERVAL '1 minute')`,
		bytes.Repeat([]byte{6}, 32), bytes.Repeat([]byte{7}, 32)); err != nil {
		t.Fatal(err)
	}
	workerReader := sdkmetric.NewManualReader()
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(workerReader)))
	t.Cleanup(func() { otel.SetMeterProvider(previousProvider) })
	runCtx, stop = context.WithCancel(ctx)
	go func() { result <- worker.Run(runCtx) }()
	waitForPurge(ctx, db)
	workerMetrics := collectMetrics(t, workerReader)
	stop()
	assertAge(t, workerMetrics, 0)
	assertLag(t, workerMetrics, 0)
	if logs.FilterMessage("identity_outbox_age").Len() != 0 || logs.FilterMessage("identity_broker_lag").Len() != 0 {
		t.Fatal("worker wrote a removed outbox age or broker lag info line")
	}
	var attempts int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM identity_provider_login_attempts`).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("expired provider attempts after the worker purge = %d, %v", attempts, err)
	}
	if err := <-result; err != nil {
		t.Fatalf("worker shutdown: %v", err)
	}
	response = httptest.NewRecorder()
	worker.health.Handler.ServeHTTP(response, httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("worker readiness after shutdown = %d", response.Code)
	}
	reader := sdkmetric.NewManualReader()
	if _, err := registerWorkerMetrics(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"), logger,
		requestTimeout, worker.outboxAge, worker.consumerLag); err != nil {
		t.Fatal(err)
	}
	collectMetrics(t, reader)
	if logs.FilterMessage("identity_outbox_age_unavailable").Len() == 0 || logs.FilterMessage("identity_broker_lag_unavailable").Len() == 0 {
		t.Fatal("worker did not record unavailable monitoring data")
	}
	worker.purgeProviderAttempts(ctx)
	if logs.FilterMessage("provider_attempt_purge_failed").Len() == 0 {
		t.Fatal("worker did not record a failed provider attempt purge")
	}
	badHealth := workerConfig
	badHealth.HealthAddress = "127.0.0.1:-1"
	failedWorker, err := NewWorker(ctx, badHealth, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := failedWorker.Run(ctx); err == nil {
		t.Fatal("worker accepted invalid health listener")
	}
	retryCtx, retryCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	failedWorker.runEmail(retryCtx)
	retryCancel()
	if logs.FilterMessage("email_worker_retry").Len() == 0 {
		t.Fatal("worker did not retry a failed delivery consumer")
	}
}

func waitForLog(t *testing.T, logs *observer.ObservedLogs, message string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if logs.FilterMessage(message).Len() > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("missing %s log", message)
		case <-ticker.C:
		}
	}
}

// waitForPurge waits up to 10 seconds until the worker removes the expired
// provider login attempts.
func waitForPurge(ctx context.Context, db *sql.DB) {
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		var attempts int
		if db.QueryRowContext(ctx, `SELECT count(*) FROM identity_provider_login_attempts`).Scan(&attempts) == nil && attempts == 0 {
			return
		}
	}
}

func applyIdentityMigration(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	goose.SetBaseFS(migrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, ".")
}
