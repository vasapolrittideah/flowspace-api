//go:build integration

package postgres_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func testCurrentSessionLogout(t *testing.T, pool *pgxpool.Pool) {
	ctx := t.Context()
	queries := identitysqlc.New(pool)
	const subject = "logout-subject"
	if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
		Subject: subject, EmailLocal: "logout", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
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
		session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: hash[:]})
		if err != nil {
			t.Fatal(err)
		}
		return uuid.UUID(session.ID.Bytes).String(), base64.RawURLEncoding.EncodeToString(secret)
	}
	currentID, currentRefresh := create(11)
	otherID, otherRefresh := create(12)
	current := inbound.LogoutCurrentSessionInput{Subject: subject, SessionID: currentID}
	repository := identitypostgres.NewSessionRepository(pool)
	logout := app.NewLogoutCurrentSessionService(repository)
	check := app.NewSessionCheckService(repository)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(privateKey, "logout-key", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	refresh := app.NewRefreshSessionService(identitypostgres.NewRefreshSessionRepository(pool), signer)
	if err := logout.LogoutCurrentSession(ctx, inbound.LogoutCurrentSessionInput{Subject: "other-subject", SessionID: currentID}); !errors.Is(err, outbound.ErrUnauthenticated) {
		t.Fatalf("wrong subject = %v", err)
	}
	rotated, err := refresh.RefreshSession(ctx, currentRefresh)
	if err != nil {
		t.Fatalf("wrong subject revoked current token: %v", err)
	}
	if err := logout.LogoutCurrentSession(ctx, current); err != nil {
		t.Fatalf("logout = %v", err)
	}
	if _, err := refresh.RefreshSession(ctx, rotated.RefreshToken); !errors.Is(err, app.ErrUnauthenticatedRefresh) {
		t.Fatalf("current device refresh after logout = %v", err)
	}
	if _, err := check.CheckSession(ctx, inbound.CheckSessionInput(current)); !errors.Is(err, outbound.ErrUnauthenticated) {
		t.Fatalf("current session check = %v", err)
	}
	if err := logout.LogoutCurrentSession(ctx, current); !errors.Is(err, outbound.ErrUnauthenticated) {
		t.Fatalf("repeated logout = %v", err)
	}
	if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: subject, SessionID: otherID}); err != nil {
		t.Fatalf("other device check = %v", err)
	}
	if _, err := refresh.RefreshSession(ctx, otherRefresh); err != nil {
		t.Fatalf("other device refresh = %v", err)
	}
	closedPool, err := pgxpool.New(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	closedPool.Close()
	if err := app.NewLogoutCurrentSessionService(identitypostgres.NewSessionRepository(closedPool)).LogoutCurrentSession(ctx, inbound.LogoutCurrentSessionInput{Subject: subject, SessionID: otherID}); !errors.Is(err, app.ErrCurrentLogoutUnavailable) {
		t.Fatalf("database failure = %v", err)
	}
}
