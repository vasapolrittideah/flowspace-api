//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
)

func TestSharedLimits(t *testing.T) {
	ctx := context.Background()
	container, err := postgrescontainer.Run(ctx, "postgres:18-alpine",
		postgrescontainer.WithDatabase("identity"),
		postgrescontainer.WithUsername("identity"),
		postgrescontainer.WithPassword("identity"),
		postgrescontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.Files)
	if err := goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	firstPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(firstPool.Close)
	secondPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondPool.Close)
	first := app.NewLimits(postgres.NewLimitRepository(firstPool))
	second := app.NewLimits(postgres.NewLimitRepository(secondPool))

	t.Run("signup retries share a rolling source window", func(t *testing.T) {
		var group sync.WaitGroup
		results := make(chan error, 12)
		for i := range 12 {
			group.Add(1)
			go func() {
				defer group.Done()
				if i%2 == 0 {
					results <- first.Signup(ctx, "192.0.2.1")
				} else {
					results <- second.Signup(ctx, "192.0.2.1")
				}
			}()
		}
		group.Wait()
		close(results)
		allowed, denied := 0, 0
		for result := range results {
			switch result {
			case nil:
				allowed++
			case app.ErrRateLimited:
				denied++
			default:
				t.Fatalf("unexpected limit result: %v", result)
			}
		}
		if allowed != 10 || denied != 2 {
			t.Fatalf("allowed %d, denied %d; want 10 and 2", allowed, denied)
		}
		if _, err := firstPool.Exec(ctx, `UPDATE identity_limit_counters SET window_start = window_start - INTERVAL '61 minutes' WHERE scope = 'source' AND counter_key = '192.0.2.1'`); err != nil {
			t.Fatal(err)
		}
		if err := second.Signup(ctx, "192.0.2.1"); err != nil {
			t.Fatalf("source did not recover after rolling window: %v", err)
		}
	})

	t.Run("code requests include missing accounts and methods", func(t *testing.T) {
		for range 60 {
			if err := first.CodeRequest(ctx, "192.0.2.2"); err != nil {
				t.Fatal(err)
			}
		}
		if err := second.CodeRequest(ctx, "192.0.2.2"); err != app.ErrRateLimited {
			t.Fatalf("request 61 = %v", err)
		}
	})

	t.Run("wrong guesses use one source bucket", func(t *testing.T) {
		for range 100 {
			if err := first.WrongCode(ctx, "192.0.2.3"); err != nil {
				t.Fatal(err)
			}
		}
		if err := second.WrongCode(ctx, "192.0.2.3"); err != app.ErrRateLimited {
			t.Fatalf("guess 101 = %v", err)
		}
	})

	t.Run("account code issues share the cooldown", func(t *testing.T) {
		if err := first.CodeIssue(ctx, "subject-1"); err != nil {
			t.Fatal(err)
		}
		if err := second.CodeIssue(ctx, "subject-1"); err != app.ErrRateLimited {
			t.Fatalf("immediate resend = %v", err)
		}
		_, err := firstPool.Exec(ctx, `UPDATE identity_limit_counters SET window_start = statement_timestamp() - INTERVAL '61 seconds' WHERE scope = 'account' AND counter_key = 'subject-1'`)
		if err != nil {
			t.Fatal(err)
		}
		for range 4 {
			if err := second.CodeIssue(ctx, "subject-1"); err != nil {
				t.Fatal(err)
			}
			_, err := firstPool.Exec(ctx, `UPDATE identity_limit_counters SET window_start = window_start - INTERVAL '61 seconds' WHERE scope = 'account' AND counter_key = 'subject-1' AND window_start > statement_timestamp() - INTERVAL '61 seconds'`)
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := first.CodeIssue(ctx, "subject-1"); err != app.ErrRateLimited {
			t.Fatalf("sixth code = %v", err)
		}
	})

	t.Run("account guesses observe daily history", func(t *testing.T) {
		for i := range 20 {
			_, err := firstPool.Exec(ctx, `INSERT INTO identity_limit_counters (scope, counter_key, action, window_start, count) VALUES ('account', 'subject-2', 'code-guess', statement_timestamp() - INTERVAL '2 hours' - $1 * INTERVAL '1 microsecond', 1)`, i)
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := first.AccountWrongCode(ctx, "subject-2"); err != app.ErrRateLimited {
			t.Fatalf("daily guess 21 = %v", err)
		}
	})

	t.Run("account guesses stop at the hourly limit", func(t *testing.T) {
		for range 10 {
			if err := first.AccountWrongCode(ctx, "subject-3"); err != nil {
				t.Fatal(err)
			}
		}
		if err := second.AccountWrongCode(ctx, "subject-3"); err != app.ErrRateLimited {
			t.Fatalf("hourly guess 11 = %v", err)
		}
	})

	t.Run("one challenge stops after five wrong guesses", func(t *testing.T) {
		queries := identitysqlc.New(firstPool)
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "subject-4", EmailLocal: "subject-4", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		challenge, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
			AccountSubject: "subject-4", Purpose: "verify-email", EmailLocal: "subject-4", EmailDomain: "example.com", CodeVerifier: bytes.Repeat([]byte{1}, 32),
		})
		if err != nil {
			t.Fatal(err)
		}
		for want := int16(1); want <= 5; want++ {
			if got, err := queries.IncrementChallengeWrongGuess(ctx, challenge.ID); err != nil || got != want {
				t.Fatalf("guess count = %d, %v; want %d", got, err, want)
			}
		}
		if _, err := queries.IncrementChallengeWrongGuess(ctx, challenge.ID); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("sixth wrong guess was accepted: %v", err)
		}
	})

	firstPool.Close()
	if err := first.Signup(ctx, "192.0.2.4"); err == nil {
		t.Fatal("unavailable limit store allowed signup")
	}
}
