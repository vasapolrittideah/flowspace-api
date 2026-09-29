//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
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
