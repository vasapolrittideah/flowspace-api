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
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
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
	exporter := recordSpans(t)
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
	testCorrelatedPrivateSessionCheck(ctx, t, private, request, logs, exporter)
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
	testProviderAccountPrivateSessionCheck(ctx, t, pool, private, signingKey)
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
	sessionLines := logs.FilterMessage("identity_session_check").Len()
	publicCtx := metadata.AppendToOutgoingContext(callCtx, "traceparent", parentTraceparent, "x-request-id", "request-public")
	if _, err := identityv1.NewIdentityServiceClient(publicConnection).CheckSession(publicCtx, request); status.Code(err) != codes.Unimplemented {
		t.Fatalf("public gRPC CheckSession = %v", err)
	}
	if got := logs.FilterMessage("identity_session_check").Len(); got != sessionLines {
		t.Fatalf("public CheckSession reached the session check: %d lines, want %d", got, sessionLines)
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

// testCorrelatedPrivateSessionCheck proves that a check over mutual TLS
// continues the forwarded trace and writes the forwarded request ID.
func testCorrelatedPrivateSessionCheck(ctx context.Context, t *testing.T, private identityv1.IdentityServiceClient,
	request *identityv1.CheckSessionRequest, logs *observer.ObservedLogs, exporter *tracetest.InMemoryExporter,
) {
	t.Helper()
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	callCtx = metadata.AppendToOutgoingContext(callCtx, "traceparent", parentTraceparent, "x-request-id", "request-1")
	response, err := private.CheckSession(callCtx, request)
	if err != nil || !response.GetEmailVerified() {
		t.Fatalf("correlated session = %+v, %v", response, err)
	}
	var continued []tracetest.SpanStub
	for _, span := range exporter.GetSpans() {
		if span.Parent.SpanID().String() == parentSpanID {
			continued = append(continued, span)
		}
	}
	if len(continued) != 1 || continued[0].Name != identityv1.IdentityService_CheckSession_FullMethodName ||
		continued[0].SpanKind != trace.SpanKindServer || continued[0].Parent.TraceID().String() != parentTraceID {
		t.Fatalf("continued spans = %+v", continued)
	}
	lines := logs.FilterMessage("identity_session_check").FilterField(zap.String("request_id", "request-1")).All()
	if len(lines) != 1 {
		t.Fatalf("identity_session_check lines with the request ID = %d", len(lines))
	}
	if fields := lines[0].ContextMap(); fields["trace_id"] != parentTraceID || fields["status"] != "OK" || fields["outcome"] != "success" {
		t.Fatalf("identity_session_check = %v", fields)
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

type fixedProviderIdentity outbound.ProviderIdentity

func (f fixedProviderIdentity) VerifyProviderIdentity(context.Context, outbound.ProviderCodeExchange) (outbound.ProviderIdentity, error) {
	return outbound.ProviderIdentity(f), nil
}

// testProviderAccountPrivateSessionCheck proves that the private check used by Workspace reports a new GitHub
// account as unverified until it uses its Flowspace code, and then reports the same subject as verified.
func testProviderAccountPrivateSessionCheck(ctx context.Context, t *testing.T, pool *pgxpool.Pool, private identityv1.IdentityServiceClient,
	signingKey ed25519.PrivateKey,
) {
	t.Helper()
	key := bytes.Repeat([]byte{2}, 32)
	signer, err := token.NewSigner(signingKey, "local-1", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{3}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	allow := func(context.Context, string) error { return nil }
	service := app.NewProviderLoginService(postgres.NewProviderAttemptRepository(pool), allow, allow, key, map[domain.Provider]app.ProviderClient{
		domain.ProviderGitHub: {
			ClientID: "github-client", CallbackURL: "http://localhost:8082/v1/provider-login-callbacks/github",
			Identity: fixedProviderIdentity{Subject: "9001", Email: "provider-session@example.com", EmailVerified: true},
		},
	}).WithSessions(postgres.NewAccountRepository(pool), signer, protector, allow)
	started, err := service.StartProviderLogin(ctx, inbound.StartProviderLoginInput{Provider: "github", Source: "192.0.2.71"})
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := service.CompleteProviderCallback(ctx, inbound.CompleteProviderCallbackInput{
		Provider: "github", State: authorization.Query().Get("state"), Code: "provider-code", Source: "192.0.2.71",
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.CreateProviderSession(ctx, inbound.CreateProviderSessionInput{AttemptToken: started.AttemptToken, HandoffCode: handoff, Source: "192.0.2.71"})
	if err != nil || session.EmailVerified {
		t.Fatalf("provider session = %q verified %t, %v", session.Subject, session.EmailVerified, err)
	}
	var sessionID, challengeID string
	var material outbound.DeliveryMaterial
	if err := pool.QueryRow(ctx, `SELECT session.id::text, challenge.id::text, delivery.key_version, delivery.nonce, delivery.ciphertext
		FROM identity_sessions AS session
		JOIN identity_challenges AS challenge ON challenge.account_subject = session.account_subject
		JOIN identity_challenge_deliveries AS delivery ON delivery.challenge_id = challenge.id
		WHERE session.account_subject = $1 AND challenge.purpose = 'verify-email'`, session.Subject).
		Scan(&sessionID, &challengeID, &material.KeyVersion, &material.Nonce, &material.Ciphertext); err != nil {
		t.Fatal(err)
	}
	request := &identityv1.CheckSessionRequest{Subject: session.Subject, SessionId: sessionID}
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if response, err := private.CheckSession(callCtx, request); err != nil || response.GetEmailVerified() {
		t.Fatalf("session check before verification = %+v, %v", response, err)
	}
	_, code, err := protector.Open(challengeID, "verify-email", session.Subject, material)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.NewEmailVerificationService(postgres.NewAccountRepository(pool), allow, allow, key).VerifyEmail(ctx, inbound.VerifyEmailInput{
		Subject: session.Subject, SessionID: sessionID, Source: "192.0.2.71", Code: code,
	}); err != nil {
		t.Fatal(err)
	}
	if response, err := private.CheckSession(callCtx, request); err != nil || !response.GetEmailVerified() {
		t.Fatalf("session check after verification = %+v, %v", response, err)
	}
	var linked string
	if err := pool.QueryRow(ctx, `SELECT account_subject FROM identity_provider_links WHERE provider = 'github' AND provider_subject = '9001'`).
		Scan(&linked); err != nil || linked != session.Subject {
		t.Fatalf("linked subject = %q, want %q: %v", linked, session.Subject, err)
	}
}
