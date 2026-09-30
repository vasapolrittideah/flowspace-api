//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

func testUnusedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func TestIdentityPrivateSessionListener(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
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
	if err := applyIdentityMigration(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash)
		VALUES ('session-subject', 'session', 'example.com', '$argon2id$test')`); err != nil {
		t.Fatal(err)
	}
	var sessionID string
	if err := pool.QueryRow(ctx, `INSERT INTO identity_sessions (account_subject, refresh_token_hash, idle_expires_at, absolute_expires_at)
		VALUES ('session-subject', $1, statement_timestamp() + INTERVAL '30 days', statement_timestamp() + INTERVAL '90 days')
		RETURNING id::text`, bytes.Repeat([]byte{1}, 32)).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	fixture := testSessionTLSFixture(t)
	keyDirectory := t.TempDir()
	_, signingKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(signingKey)
	if err != nil {
		t.Fatal(err)
	}
	signingFile := filepath.Join(keyDirectory, "signing.pem")
	verifierFile := filepath.Join(keyDirectory, "verifier")
	deliveryFile := filepath.Join(keyDirectory, "delivery")
	for path, data := range map[string][]byte{
		signingFile:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}),
		verifierFile: bytes.Repeat([]byte{2}, 32), deliveryFile: bytes.Repeat([]byte{3}, 32),
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := fixture.config
	config.Environment = "local"
	config.HTTPAddress = testUnusedAddress(t)
	config.InternalHTTPAddress = testUnusedAddress(t)
	config.SessionGRPCAddress = testUnusedAddress(t)
	config.DatabaseURL = sharedconfig.Secret(dsn)
	config.SigningKeyFile, config.SigningKeyID = signingFile, "local-1"
	config.TokenIssuer, config.TokenAudience = "urn:flowspace:identity:local", "flowspace-api"
	config.CodeVerifierKeyFile, config.DeliveryKeyFile = verifierFile, deliveryFile
	config.OutboxReadyMaxPending = 10000
	core, logs := observer.New(zap.InfoLevel)
	server, err := NewAPIServer(ctx, config, zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { result <- server.Run(runCtx) }()
	t.Cleanup(func() {
		stop()
		if err := <-result; err != nil {
			t.Errorf("API shutdown: %v", err)
		}
	})
	waitForLog(t, logs, "process_listening")
	clientTLS := &tls.Config{
		RootCAs: fixture.roots, ServerName: sessionTestServerName, MinVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{fixture.clients["approved"]},
	}
	privateConnection, err := grpc.NewClient("passthrough:///"+config.SessionGRPCAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = privateConnection.Close() }()
	private := identityv1.NewIdentityServiceClient(privateConnection)
	request := &identityv1.CheckSessionRequest{Subject: "session-subject", SessionId: sessionID}
	callCtx, callCancel := context.WithTimeout(ctx, 3*time.Second)
	defer callCancel()
	response, err := private.CheckSession(callCtx, request)
	if err != nil || response.GetEmailVerified() {
		t.Fatalf("initial session = %+v, %v", response, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET email_verified_at = statement_timestamp() WHERE subject = 'session-subject'`); err != nil {
		t.Fatal(err)
	}
	response, err = private.CheckSession(callCtx, request)
	if err != nil || !response.GetEmailVerified() {
		t.Fatalf("current verification = %+v, %v", response, err)
	}
	for _, test := range []struct {
		name    string
		request *identityv1.CheckSessionRequest
	}{
		{"wrong subject", &identityv1.CheckSessionRequest{Subject: "other-subject", SessionId: sessionID}},
		{"unknown session", &identityv1.CheckSessionRequest{Subject: "session-subject", SessionId: "00000000-0000-0000-0000-000000000000"}},
	} {
		_, err := private.CheckSession(callCtx, test.request)
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s = %v", test.name, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET revoked_at = statement_timestamp() WHERE id = $1`, sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := private.CheckSession(callCtx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("revoked session = %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET revoked_at = NULL, idle_expires_at = statement_timestamp() WHERE id = $1`, sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := private.CheckSession(callCtx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expired session = %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET idle_expires_at = statement_timestamp() + INTERVAL '30 days' WHERE id = $1`, sessionID); err != nil {
		t.Fatal(err)
	}
	testPasswordResetRevokesPrivateSessionCheck(ctx, t, pool, private)
	if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET email_verified_at = NULL, retired_at = statement_timestamp() WHERE subject = 'session-subject'`); err != nil {
		t.Fatal(err)
	}
	if _, err := private.CheckSession(callCtx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("retired account = %v", err)
	}
	publicConnection, err := grpc.NewClient("passthrough:///"+config.HTTPAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = publicConnection.Close() }()
	if _, err := identityv1.NewIdentityServiceClient(publicConnection).CheckSession(callCtx, request); status.Code(err) != codes.Unimplemented {
		t.Fatalf("public gRPC CheckSession = %v", err)
	}
	httpClient := &http.Client{Timeout: 3 * time.Second}
	for path, want := range map[string]int{
		"http://" + config.InternalHTTPAddress + "/livez":                 http.StatusOK,
		"http://" + config.InternalHTTPAddress + "/.well-known/jwks.json": http.StatusOK,
		"http://" + config.HTTPAddress + "/v1/check-session":              http.StatusNotFound,
	} {
		response, err := httpClient.Get(path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("%s = %d, want %d", path, response.StatusCode, want)
		}
	}
}

// testPasswordResetRevokesPrivateSessionCheck proves that the private check used by Workspace rejects a session after reset.
func testPasswordResetRevokesPrivateSessionCheck(ctx context.Context, t *testing.T, pool *pgxpool.Pool, private identityv1.IdentityServiceClient) {
	t.Helper()
	const subject, email = "reset-session-subject", "reset@example.com"
	if _, err := pool.Exec(ctx, `INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash, email_verified_at)
		VALUES ($1, 'reset', 'example.com', '$argon2id$test', statement_timestamp())`, subject); err != nil {
		t.Fatal(err)
	}
	var sessionID string
	if err := pool.QueryRow(ctx, `INSERT INTO identity_sessions (account_subject, refresh_token_hash, idle_expires_at, absolute_expires_at)
		VALUES ($1, $2, statement_timestamp() + INTERVAL '30 days', statement_timestamp() + INTERVAL '90 days')
		RETURNING id::text`, subject, bytes.Repeat([]byte{5}, 32)).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{2}, 32)
	code, verifier, _, err := domain.NewChallenge(key, subject, email, domain.PurposePasswordReset, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity_challenges (account_subject, purpose, email_local, email_domain, code_verifier, expires_at)
		VALUES ($1, 'password-reset', 'reset', 'example.com', $2, statement_timestamp() + INTERVAL '10 minutes')`, subject, verifier[:]); err != nil {
		t.Fatal(err)
	}
	request := &identityv1.CheckSessionRequest{Subject: subject, SessionId: sessionID}
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := private.CheckSession(callCtx, request); err != nil {
		t.Fatalf("session before reset = %v", err)
	}
	allow := func(context.Context, string) error { return nil }
	reset := app.NewPasswordResetService(postgres.NewAccountRepository(pool), allow, allow, allow,
		func(context.Context, string) (bool, error) { return false, nil }, key)
	if err := reset.ResetPassword(ctx, inbound.ResetPasswordInput{
		Email: email, Code: code, NewPassword: "reset session password 123", Source: "192.0.2.70",
	}); err != nil {
		t.Fatalf("reset = %v", err)
	}
	if _, err := private.CheckSession(callCtx, request); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("session after reset = %v", err)
	}
}
