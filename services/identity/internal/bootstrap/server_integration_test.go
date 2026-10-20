//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"

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
	apiReader := sdkmetric.NewManualReader()
	apiMetrics := sdkmetric.NewMeterProvider(sdkmetric.WithReader(apiReader))
	oldMetrics := otel.GetMeterProvider()
	otel.SetMeterProvider(apiMetrics)
	t.Cleanup(func() { otel.SetMeterProvider(oldMetrics); _ = apiMetrics.Shutdown(context.Background()) })
	spans := recordSpans(t)
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
	proofToken, proofSession, proofCode := testPasswordReauthenticationAPI(t, server, dsn, privateKey)
	var apiData metricdata.ResourceMetrics
	if err := apiReader.Collect(ctx, &apiData); err != nil {
		t.Fatal(err)
	}
	observed := fmt.Sprint(logs.All(), spans.GetSpans(), apiData)
	if !strings.Contains(observed, "/v1/password-reauthentications") {
		t.Fatal("missing reauthentication telemetry")
	}
	for _, secret := range []string{"api-reauth@example.com", "correct café battery staple", "wrong reauth password", proofToken} {
		if strings.Contains(observed, secret) {
			t.Fatal("telemetry exposed credential data")
		}
	}
	codeFields := []any{}
	for _, entry := range logs.All() {
		codeFields = append(codeFields, entry.Message)
		for name, value := range entry.ContextMap() {
			if name != "trace_id" && name != "request_id" {
				codeFields = append(codeFields, logStringValues(value))
			}
		}
	}
	for _, span := range spans.GetSpans() {
		codeFields = append(codeFields, span.Name, span.Attributes, span.Status)
		for _, event := range span.Events {
			codeFields = append(codeFields, event.Name, event.Attributes)
		}
	}
	for _, scope := range apiData.ScopeMetrics {
		for _, measured := range scope.Metrics {
			if histogram, ok := measured.Data.(metricdata.Histogram[float64]); ok {
				for _, point := range histogram.DataPoints {
					codeFields = append(codeFields, point.Attributes)
				}
			}
		}
	}
	if strings.Contains(fmt.Sprint(codeFields...), proofCode) {
		t.Fatal("telemetry exposed re-authentication code")
	}
	metricText := fmt.Sprint(apiData)
	for _, identifier := range []string{"api-reauth", proofSession} {
		if strings.Contains(metricText, identifier) {
			t.Fatal("metric attributes exposed an identity")
		}
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

func logStringValues(value any) []string {
	switch value := value.(type) {
	case string:
		return []string{value}
	case []string:
		return value
	case map[string]any:
		var values []string
		for _, nested := range value {
			values = append(values, logStringValues(nested)...)
		}
		return values
	case []any:
		var values []string
		for _, nested := range value {
			values = append(values, logStringValues(nested)...)
		}
		return values
	default:
		return nil
	}
}

func testPasswordReauthenticationAPI(t *testing.T, server *APIServer, dsn string, key ed25519.PrivateKey) (string, string, string) {
	t.Helper()
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	password := "correct café battery staple"
	hash, err := domain.HashPassword(ctx, password, func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	queries := identitysqlc.New(pool)
	if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{Subject: "api-reauth", EmailLocal: "api-reauth", EmailDomain: "example.com", PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("api-reauth"))
	session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: "api-reauth", RefreshTokenHash: digest[:]})
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.UUID(session.ID.Bytes).String()
	if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=NULL WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(key, "local-1", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	access, err := signer.Sign(outbound.AccessTokenClaims{Subject: "api-reauth", SessionID: id, IssuedAt: time.Now(), ExpiresAt: time.Now().Add(5 * time.Minute), ID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	call := func(body, authorization string, want int) []byte {
		t.Helper()
		request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/password-reauthentications", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", authorization)
		request.RemoteAddr = "192.0.2.215:42"
		response := httptest.NewRecorder()
		server.public.Handler.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatalf("password reauthentication REST=%d body=%s", response.Code, response.Body)
		}
		if strings.Contains(response.Body.String(), access) {
			t.Fatal("error/response exposed bearer token")
		}
		return response.Body.Bytes()
	}
	call(`{"password":"wrong reauth password"}`, "Bearer "+access, 400)
	call(`{}`, "Bearer "+access, 400)
	call(`{"password":"correct café battery staple"}`, "", 401)
	body := call(`{"password":"correct café battery staple"}`, "Bearer "+access, 200)
	var response struct {
		ExpiresAt time.Time `json:"reauthenticationExpiresAt"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.ExpiresAt.IsZero() {
		t.Fatalf("expiry absent: %v", err)
	}
	if next := call(`{"password":"correct café battery staple"}`, "Bearer "+access, 200); !bytes.Equal(body, next) {
		t.Fatal("REST retry moved expiry")
	}

	var code string
	callCode := func(path, body, authorization string, want int) []byte {
		t.Helper()
		request := httptest.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", authorization)
		request.RemoteAddr = "192.0.2.215:42"
		response := httptest.NewRecorder()
		server.public.Handler.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body)
		}
		if strings.Contains(response.Body.String(), access) || code != "" && want >= 400 && strings.Contains(response.Body.String(), code) {
			t.Fatal("code response exposed a credential")
		}
		return response.Body.Bytes()
	}
	callCode("/v1/reauthentication-codes", `{}`, "Bearer "+access, 400)
	callCode("/v1/reauthentication-codes", `{}`, "", 401)
	if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET email_verified_at=statement_timestamp() WHERE subject='api-reauth'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=NULL WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	accepted := callCode("/v1/reauthentication-codes", `{}`, "Bearer "+access, 200)
	if !bytes.Contains(accepted, []byte(`"accepted":true`)) {
		t.Fatal("code request was not accepted")
	}
	var challengeID string
	var material outbound.DeliveryMaterial
	if err := pool.QueryRow(ctx, `SELECT challenge.id::text,delivery.key_version,delivery.nonce,delivery.ciphertext FROM identity_challenges challenge JOIN identity_challenge_deliveries delivery ON delivery.challenge_id=challenge.id WHERE challenge.account_subject='api-reauth' AND challenge.purpose='reauthenticate'`).Scan(&challengeID, &material.KeyVersion, &material.Nonce, &material.Ciphertext); err != nil {
		t.Fatal(err)
	}
	opener, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{2}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	_, code, err = opener.Open(challengeID, "reauthenticate", "api-reauth", material)
	if err != nil {
		t.Fatal(err)
	}
	callCode("/v1/code-reauthentications", `{"code":"invalid"}`, "Bearer "+access, 400)
	callCode("/v1/code-reauthentications", `{"code":"`+code+`"}`, "", 401)
	codeProof := callCode("/v1/code-reauthentications", `{"code":"`+code+`"}`, "Bearer "+access, 200)
	if err := json.Unmarshal(codeProof, &response); err != nil || response.ExpiresAt.IsZero() {
		t.Fatal("code response missing expiry")
	}
	callCode("/v1/code-reauthentications", `{"code":"`+code+`"}`, "Bearer "+access, 400)
	if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	call(`{"password":"correct café battery staple"}`, "Bearer "+access, 401)
	return access, id, code
}
