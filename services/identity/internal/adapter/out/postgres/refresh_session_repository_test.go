//go:build integration

package postgres_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

func testRefreshSessionRepository(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	queries := identitysqlc.New(pool)
	if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
		Subject: "refresh-subject", EmailLocal: "refresh", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
	}); err != nil {
		t.Fatal(err)
	}
	create := func(seed byte) (string, string) {
		t.Helper()
		secret := make([]byte, 32)
		for i := range secret {
			secret[i] = seed
		}
		hash := sha256.Sum256(secret)
		session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: "refresh-subject", RefreshTokenHash: hash[:]})
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(secret), uuid.UUID(session.ID.Bytes).String()
	}
	first, firstID := create(1)
	other, otherID := create(2)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(privateKey, "refresh-key", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := token.NewVerifier(publicKey, "refresh-key", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	service := app.NewSessionRefreshService(identitypostgres.NewSessionRefreshRepository(pool), signer)
	result, err := service.RefreshSession(ctx, first)
	if err != nil || result.RefreshToken == first || result.AccessToken == "" {
		t.Fatalf("rotation = %+v, %v", result, err)
	}
	identity, err := verifier.Verify(result.AccessToken)
	if err != nil || identity.SessionID != firstID || identity.Subject != "refresh-subject" {
		t.Fatalf("claims = %+v, %v", identity, err)
	}
	var absolute time.Time
	if err := pool.QueryRow(ctx, `SELECT absolute_expires_at FROM identity_sessions WHERE id = $1`, firstID).Scan(&absolute); err != nil {
		t.Fatal(err)
	}
	if !result.SessionExpiresAt.Equal(absolute) || !result.RefreshTokenExpiresAt.After(time.Now().Add(29*24*time.Hour)) {
		t.Fatal("wrong session expiry")
	}
	if _, err := service.RefreshSession(ctx, first); !errors.Is(err, app.ErrUnauthenticatedRefresh) {
		t.Fatalf("replay = %v", err)
	}
	if _, err := service.RefreshSession(ctx, result.RefreshToken); !errors.Is(err, app.ErrUnauthenticatedRefresh) {
		t.Fatalf("revoked current token = %v", err)
	}
	check := app.NewSessionCheckService(identitypostgres.NewSessionRepository(pool))
	if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: "refresh-subject", SessionID: firstID}); err == nil {
		t.Fatal("replayed session remained active")
	}
	if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: "refresh-subject", SessionID: otherID}); err != nil {
		t.Fatalf("other device revoked: %v", err)
	}
	if _, err := service.RefreshSession(ctx, base64.RawURLEncoding.EncodeToString(make([]byte, 32))); !errors.Is(err, app.ErrUnauthenticatedRefresh) {
		t.Fatalf("unknown token = %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() { defer workers.Done(); <-start; _, err := service.RefreshSession(ctx, other); results <- err }()
	}
	close(start)
	workers.Wait()
	close(results)
	success, rejected := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, app.ErrUnauthenticatedRefresh):
			rejected++
		default:
			t.Fatalf("concurrent refresh = %v", err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("concurrent results = %d success, %d rejected", success, rejected)
	}
	if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: "refresh-subject", SessionID: otherID}); err == nil {
		t.Fatal("concurrent replay did not revoke")
	}

	short, shortID := create(3)
	if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET idle_expires_at = statement_timestamp() + INTERVAL '5 minutes', absolute_expires_at = statement_timestamp() + INTERVAL '5 minutes' WHERE id = $1`, shortID); err != nil {
		t.Fatal(err)
	}
	shortResult, err := service.RefreshSession(ctx, short)
	if err != nil || !shortResult.RefreshTokenExpiresAt.Equal(shortResult.SessionExpiresAt) ||
		!shortResult.AccessTokenExpiresAt.Equal(shortResult.SessionExpiresAt.Truncate(time.Second)) {
		t.Fatalf("absolute cap = %+v, %v", shortResult, err)
	}

	rollback, _ := create(4)
	failing := app.NewSessionRefreshService(identitypostgres.NewSessionRefreshRepository(pool), failingTokenSigner{})
	if _, err := failing.RefreshSession(ctx, rollback); !errors.Is(err, app.ErrRefreshUnavailable) {
		t.Fatalf("signing failure = %v", err)
	}
	if _, err := service.RefreshSession(ctx, rollback); err != nil {
		t.Fatalf("signing failure consumed token: %v", err)
	}
	unavailablePool, err := pgxpool.New(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	unavailablePool.Close()
	unavailable := app.NewSessionRefreshService(identitypostgres.NewSessionRefreshRepository(unavailablePool), signer)
	if _, err := unavailable.RefreshSession(ctx, shortResult.RefreshToken); !errors.Is(err, app.ErrRefreshUnavailable) {
		t.Fatalf("store failure = %v", err)
	}

	for _, expiry := range []string{"idle_expires_at", "absolute_expires_at"} {
		current, id := create(byte(len(expiry)))
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET idle_expires_at = statement_timestamp(), absolute_expires_at = CASE WHEN $1 = 'absolute_expires_at' THEN statement_timestamp() ELSE absolute_expires_at END WHERE id = $2`, expiry, id); err != nil {
			t.Fatal(err)
		}
		if _, err := service.RefreshSession(ctx, current); !errors.Is(err, app.ErrUnauthenticatedRefresh) {
			t.Fatalf("%s boundary = %v", expiry, err)
		}
	}
}
