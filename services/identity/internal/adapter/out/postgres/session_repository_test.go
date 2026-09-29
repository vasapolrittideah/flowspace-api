//go:build integration

package postgres_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func testSessionIssuanceRollback(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Run("session issuance stores only a hash and rolls back with account", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := identitysqlc.New(tx).CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "issue-rollback", EmailLocal: "issue-rollback", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		service := app.NewSessionService(identitypostgres.NewSessionRepository(tx), failingTokenSigner{})
		if _, err := service.Issue(ctx, "issue-rollback"); !errors.Is(err, app.ErrSessionIssue) {
			t.Fatalf("signer failure = %v", err)
		}
		var hash []byte
		if err := tx.QueryRow(ctx, `SELECT refresh_token_hash FROM identity_sessions WHERE account_subject = $1`, "issue-rollback").Scan(&hash); err != nil || len(hash) != 32 {
			t.Fatalf("stored refresh hash = %x, %v", hash, err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		for _, check := range []struct{ name, query string }{
			{"account", `SELECT count(*) FROM identity_accounts WHERE subject = 'issue-rollback'`},
			{"session", `SELECT count(*) FROM identity_sessions WHERE account_subject = 'issue-rollback'`},
		} {
			var count int
			if err := pool.QueryRow(ctx, check.query).Scan(&count); err != nil || count != 0 {
				t.Fatalf("%s after rollback = %d, %v", check.name, count, err)
			}
		}
	})
}

func testSessionCheckRepository(t *testing.T, ctx context.Context, pool *pgxpool.Pool, dsn string) {
	t.Run("session check reads current account and session state", func(t *testing.T) {
		queries := identitysqlc.New(pool)
		create := func(subject string) (string, pgtype.UUID) {
			t.Helper()
			if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
				Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test",
			}); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256([]byte(subject))
			session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: hash[:]})
			if err != nil {
				t.Fatal(err)
			}
			return uuid.UUID(session.ID.Bytes).String(), session.ID
		}
		service := app.NewSessionCheckService(identitypostgres.NewSessionRepository(pool))
		check := func(subject, sessionID string) (bool, error) {
			return service.CheckSession(ctx, inbound.CheckSessionInput{Subject: subject, SessionID: sessionID})
		}
		activeID, activeUUID := create("check-active")
		if verified, err := check("check-active", activeID); err != nil || verified {
			t.Fatalf("unverified session = %t, %v", verified, err)
		}
		if _, err := queries.MarkEmailVerified(ctx, "check-active"); err != nil {
			t.Fatal(err)
		}
		if verified, err := check("check-active", activeID); err != nil || !verified {
			t.Fatalf("verified session = %t, %v", verified, err)
		}
		for _, input := range []inbound.CheckSessionInput{
			{Subject: "wrong", SessionID: activeID},
			{Subject: "check-active", SessionID: uuid.NewString()},
			{Subject: "check-active", SessionID: "invalid-uuid"},
		} {
			verified, err := check(input.Subject, input.SessionID)
			if verified || !errors.Is(err, outbound.ErrUnauthenticated) {
				t.Fatalf("wrong identifiers %+v = %t, %v", input, verified, err)
			}
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET revoked_at = statement_timestamp() WHERE id = $1`, activeUUID); err != nil {
			t.Fatal(err)
		}
		if verified, err := check("check-active", activeID); verified || !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatalf("revoked session = %t, %v", verified, err)
		}

		for _, test := range []struct{ subject, update string }{
			{"check-idle", `UPDATE identity_sessions SET idle_expires_at = statement_timestamp() WHERE id = $1`},
			{"check-absolute", `UPDATE identity_sessions SET idle_expires_at = statement_timestamp(), absolute_expires_at = statement_timestamp() WHERE id = $1`},
			{"check-retired", `UPDATE identity_accounts SET retired_at = statement_timestamp() WHERE subject = $1`},
		} {
			id, sessionUUID := create(test.subject)
			argument := any(sessionUUID)
			if test.subject == "check-retired" {
				argument = test.subject
			}
			if _, err := pool.Exec(ctx, test.update, argument); err != nil {
				t.Fatal(err)
			}
			if verified, err := check(test.subject, id); verified || !errors.Is(err, outbound.ErrUnauthenticated) {
				t.Fatalf("%s = %t, %v", test.subject, verified, err)
			}
		}

		unavailablePool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		unavailablePool.Close()
		unavailable := app.NewSessionCheckService(identitypostgres.NewSessionRepository(unavailablePool))
		verified, err := unavailable.CheckSession(ctx, inbound.CheckSessionInput{Subject: "check-active", SessionID: activeID})
		if verified || !errors.Is(err, app.ErrSessionCheckUnavailable) {
			t.Fatalf("database failure = %t, %v", verified, err)
		}
	})
}

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
	logout := app.NewCurrentSessionLogoutService(repository)
	check := app.NewSessionCheckService(repository)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(privateKey, "logout-key", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	refresh := app.NewSessionRefreshService(identitypostgres.NewSessionRefreshRepository(pool), signer)
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
	if err := app.NewCurrentSessionLogoutService(identitypostgres.NewSessionRepository(closedPool)).LogoutCurrentSession(ctx, inbound.LogoutCurrentSessionInput{Subject: subject, SessionID: otherID}); !errors.Is(err, app.ErrCurrentLogoutUnavailable) {
		t.Fatalf("database failure = %v", err)
	}
}
