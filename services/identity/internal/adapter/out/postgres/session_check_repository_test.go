//go:build integration

package postgres_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

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
