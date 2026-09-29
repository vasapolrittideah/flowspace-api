//go:build integration

package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/db/migrations"
	identityadapter "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/identity"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

const integrationIssuer = "urn:flowspace:identity:test"

func TestServerStartsAndShutsDownWithDependencies(t *testing.T) {
	ctx := t.Context()
	container, err := postgrescontainer.Run(
		ctx,
		"postgres:18-alpine",
		postgrescontainer.WithDatabase("workspace"),
		postgrescontainer.WithUsername("workspace"),
		postgrescontainer.WithPassword("workspace"),
		postgrescontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Error(err)
		}
	})

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable", "application_name=workspace-api")
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile, caFile, _, _ := testIdentityTLSFiles(t)
	jwksURL := testJWKSURL(t)
	failedDatabaseURL, err := container.ConnectionString(ctx, "sslmode=disable", "application_name=workspace-api-failed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewServer(ctx, Config{
		HTTPAddress:     "127.0.0.1:0",
		DatabaseURL:     sharedconfig.Secret(failedDatabaseURL),
		IdentityJWKSURL: jwksURL,
		IdentityIssuer:  integrationIssuer, IdentityAudience: "flowspace-api",
		IdentitySessionAddress: "identity.test:8082", IdentitySessionServerName: "identity.test",
		IdentityClientCertFile: certFile, IdentityClientKeyFile: keyFile, IdentityCAFile: caFile,
		OIDCDiscoveryURL: "http://keycloak/missing",
	}, zap.NewNop()); err == nil {
		t.Fatal("NewServer() accepted failed OIDC discovery")
	}
	if _, err := NewServer(ctx, Config{
		DatabaseURL: sharedconfig.Secret(failedDatabaseURL), IdentityJWKSURL: jwksURL,
		IdentityIssuer: integrationIssuer, IdentityAudience: "flowspace-api",
		IdentitySessionAddress: "identity.test:8082", IdentitySessionServerName: "identity.test",
		IdentityClientCertFile: certFile + ".missing", IdentityClientKeyFile: keyFile, IdentityCAFile: caFile,
	}, zap.NewNop()); err == nil {
		t.Fatal("NewServer() accepted missing client certificate")
	}
	assertNoDatabaseConnections(t, container, "workspace-api-failed")

	address := freeAddress(t)
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core, zap.Fields(zap.String("service", "workspace-api"), zap.String("environment", "integration")))
	server, err := NewServer(ctx, Config{
		Environment:     "integration",
		HTTPAddress:     address,
		DatabaseURL:     sharedconfig.Secret(databaseURL),
		IdentityJWKSURL: jwksURL,
		IdentityIssuer:  integrationIssuer, IdentityAudience: "flowspace-api",
		IdentitySessionAddress: "identity.test:8082", IdentitySessionServerName: "identity.test",
		IdentityClientCertFile: certFile, IdentityClientKeyFile: keyFile, IdentityCAFile: caFile,
	}, logger)
	if err != nil {
		t.Fatal(err)
	}

	runContext, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { result <- server.Run(runContext) }()
	waitForServer(t, result, "http://"+address+"/v1/workspaces/workspace-1")
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(serverTimeout):
		t.Fatal("server did not stop before the shutdown deadline")
	}

	entries := logs.FilterMessage("process_listening").AllUntimed()
	if len(entries) != 1 || entries[0].ContextMap()["service"] != "workspace-api" || entries[0].ContextMap()["environment"] != "integration" {
		t.Fatalf("process_listening logs = %v", entries)
	}
	assertNoDatabaseConnections(t, container, "workspace-api")
}

func testJWKSURL(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(response).Encode(map[string]any{"keys": []any{}}); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func testIdentityTLSFiles(t *testing.T) (string, string, string, tls.Certificate, *x509.CertPool) {
	t.Helper()
	_, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	issue := func(serial int64, usage x509.ExtKeyUsage) (tls.Certificate, []byte) {
		t.Helper()
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial), DNSNames: []string{"identity.test"},
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, key.Public(), caKey)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, keyDER
	}
	client, keyDER := issue(2, x509.ExtKeyUsageClientAuth)
	server, _ := issue(3, x509.ExtKeyUsageServerAuth)
	directory := t.TempDir()
	certFile, keyFile, caFile := filepath.Join(directory, "client.crt"), filepath.Join(directory, "client.key"), filepath.Join(directory, "ca.crt")
	for path, content := range map[string][]byte{
		certFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: client.Certificate[0]}),
		keyFile:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		caFile:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
	} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return certFile, keyFile, caFile, server, pool
}

func freeAddress(t *testing.T) string {
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

func waitForServer(t *testing.T, result <-chan error, url string) {
	t.Helper()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.Now().Add(serverTimeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-result:
			t.Fatalf("server stopped before it accepted requests: %v", err)
		default:
		}
		response, err := client.Get(url)
		if err == nil {
			_ = response.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not accept requests before the startup deadline")
}

func assertNoDatabaseConnections(t *testing.T, container *postgrescontainer.PostgresContainer, applicationName string) {
	t.Helper()
	ctx := t.Context()
	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE application_name = $1", applicationName).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("database connections for %q = %d, want 0", applicationName, count)
	}
}

type admissionSessionServer struct {
	identityv1.UnimplementedIdentityServiceServer
	state *atomic.Int32
}

func (s admissionSessionServer) CheckSession(_ context.Context, request *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
	if request.GetSessionId() != "session-1" || request.GetSubject() == "retired-subject" {
		return nil, status.Error(codes.Unauthenticated, "inactive")
	}
	if request.GetSubject() == "subject-2" {
		return &identityv1.CheckSessionResponse{EmailVerified: true}, nil
	}
	if request.GetSubject() != "subject-1" {
		return nil, status.Error(codes.Unauthenticated, "inactive")
	}
	switch s.state.Load() {
	case 0:
		return &identityv1.CheckSessionResponse{EmailVerified: false}, nil
	case 1:
		return &identityv1.CheckSessionResponse{EmailVerified: true}, nil
	case 2:
		return nil, status.Error(codes.Unauthenticated, "inactive")
	default:
		return nil, status.Error(codes.Unavailable, "offline")
	}
}

func TestWorkspaceAdmissionUsesLocalTokenAndLiveIdentityState(t *testing.T) {
	ctx := t.Context()
	container, err := postgrescontainer.Run(ctx, "postgres:18-alpine",
		postgrescontainer.WithDatabase("workspace"), postgrescontainer.WithUsername("workspace"),
		postgrescontainer.WithPassword("workspace"), postgrescontainer.BasicWaitStrategies())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Error(err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.Files)
	if err := goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: public, KeyID: "key-1", Algorithm: "EdDSA", Use: "sig"}}})
	}))
	defer jwks.Close()
	certFile, keyFile, caFile, serverCert, roots := testIdentityTLSFiles(t)
	sessionListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	state := new(atomic.Int32)
	sessionServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots,
	})))
	identityv1.RegisterIdentityServiceServer(sessionServer, admissionSessionServer{state: state})
	go func() { _ = sessionServer.Serve(sessionListener) }()
	defer sessionServer.Stop()

	address := freeAddress(t)
	workspace, err := NewServer(ctx, Config{
		Environment: "integration", HTTPAddress: address, DatabaseURL: sharedconfig.Secret(dsn),
		IdentityJWKSURL: jwks.URL, IdentityIssuer: integrationIssuer, IdentityAudience: "flowspace-api",
		IdentitySessionAddress: sessionListener.Addr().String(), IdentitySessionServerName: "identity.test",
		IdentityClientCertFile: certFile, IdentityClientKeyFile: keyFile, IdentityCAFile: caFile,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { result <- workspace.Run(runCtx) }()
	waitForServer(t, result, "http://"+address+"/v1/workspaces/invalid")
	defer func() {
		cancel()
		if err := <-result; err != nil {
			t.Error(err)
		}
	}()

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: jose.JSONWebKey{Key: private, KeyID: "key-1"}}, (&jose.SignerOptions{}).WithType("at+jwt"))
	if err != nil {
		t.Fatal(err)
	}
	sign := func(subject, audience string, expires time.Time) string {
		t.Helper()
		token, err := jwt.Signed(signer).Claims(jwt.Claims{
			Issuer: integrationIssuer, Subject: subject, Audience: jwt.Audience{audience},
			IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)), Expiry: jwt.NewNumericDate(expires), ID: "token-1",
		}).Claims(struct {
			SessionID string `json:"sid"`
		}{SessionID: "session-1"}).Serialize()
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	token := sign("subject-1", "flowspace-api", time.Now().Add(time.Minute))
	call := func(method, path, body, bearer string) *http.Response {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, method, "http://"+address+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+bearer)
		if method == http.MethodPost {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "create-1")
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	assertStatus := func(response *http.Response, want int) {
		t.Helper()
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, want, body)
		}
	}
	assertStatus(call(http.MethodPost, "/v1/workspaces", `{"name":"Team"}`, token), http.StatusForbidden)
	state.Store(1)
	created := call(http.MethodPost, "/v1/workspaces", `{"name":"Team"}`, token)
	if created.StatusCode != http.StatusOK {
		assertStatus(created, http.StatusOK)
	}
	var payload struct {
		Workspace struct {
			ID string `json:"id"`
		} `json:"workspace"`
	}
	createdBody, err := io.ReadAll(created.Body)
	_ = created.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(createdBody, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Workspace.ID == "" {
		t.Fatal("verified session created no workspace")
	}
	path := "/v1/workspaces/" + payload.Workspace.ID
	assertStatus(call(http.MethodGet, path, "", token), http.StatusOK)
	assertStatus(call(http.MethodGet, path, "", sign("subject-2", "flowspace-api", time.Now().Add(time.Minute))), http.StatusNotFound)
	assertStatus(call(http.MethodGet, path, "", sign("retired-subject", "flowspace-api", time.Now().Add(time.Minute))), http.StatusUnauthorized)
	assertStatus(call(http.MethodGet, path, "", sign("subject-1", "wrong-audience", time.Now().Add(time.Minute))), http.StatusUnauthorized)
	assertStatus(call(http.MethodGet, path, "", sign("subject-1", "flowspace-api", time.Now().Add(-time.Second))), http.StatusUnauthorized)
	state.Store(2)
	assertStatus(call(http.MethodGet, path, "", token), http.StatusUnauthorized)
	state.Store(3)
	assertStatus(call(http.MethodGet, path, "", token), http.StatusServiceUnavailable)
	state.Store(1)
	wrongServer, err := identityadapter.NewTokenVerifier(identityadapter.Config{
		JWKSURL: jwks.URL, Issuer: integrationIssuer, Audience: "flowspace-api",
		SessionAddress: sessionListener.Addr().String(), SessionServerName: "wrong.test",
		ClientCertFile: certFile, ClientKeyFile: keyFile, CAFile: caFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = wrongServer.Close() }()
	if _, err := wrongServer.VerifyToken(ctx, token); !errors.Is(err, outbound.ErrIdentityUnavailable) {
		t.Fatalf("wrong server name error = %v, want identity unavailable", err)
	}
}
