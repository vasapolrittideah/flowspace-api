//go:build integration

package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	_ "github.com/jackc/pgx/v5/stdlib"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/db/migrations"
	identityadapter "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/identity"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

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
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != want {
			body, _ := io.ReadAll(response.Body)
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
	if err := json.NewDecoder(created.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	_ = created.Body.Close()
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
