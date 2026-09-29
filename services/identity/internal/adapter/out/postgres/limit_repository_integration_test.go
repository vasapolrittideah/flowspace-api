//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
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
	first := app.NewLimitService(postgres.NewLimitRepository(firstPool))
	second := app.NewLimitService(postgres.NewLimitRepository(secondPool))

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

	t.Run("password login source limit is shared and denied requests count nowhere", func(t *testing.T) {
		for i := range 60 {
			limits := first
			if i%2 == 1 {
				limits = second
			}
			if err := limits.PasswordLogin(ctx, "192.0.2.50", fmt.Sprintf("source-%d@example.com", i)); err != nil {
				t.Fatalf("source attempt %d: %v", i+1, err)
			}
		}
		if err := second.PasswordLogin(ctx, "192.0.2.50", "denied@example.com"); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("source attempt 61 = %v", err)
		}
		var sourceCount, deniedEmailCount int
		if err := firstPool.QueryRow(ctx, `SELECT COALESCE(SUM(count), 0) FROM identity_limit_counters WHERE scope = 'source' AND action = 'password-login' AND counter_key = '192.0.2.50'`).Scan(&sourceCount); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte("denied@example.com"))
		if err := firstPool.QueryRow(ctx, `SELECT COALESCE(SUM(count), 0) FROM identity_limit_counters WHERE scope = 'email' AND action = 'password-login' AND counter_key = $1`, hex.EncodeToString(digest[:])).Scan(&deniedEmailCount); err != nil {
			t.Fatal(err)
		}
		if sourceCount != 60 || deniedEmailCount != 0 {
			t.Fatalf("denied source counts = %d source, %d email; want 60 and 0", sourceCount, deniedEmailCount)
		}
		if _, err := firstPool.Exec(ctx, `UPDATE identity_limit_counters SET window_start = statement_timestamp() - INTERVAL '59 minutes' WHERE scope = 'source' AND action = 'password-login' AND counter_key = '192.0.2.50' AND window_start = (SELECT MIN(window_start) FROM identity_limit_counters WHERE scope = 'source' AND action = 'password-login' AND counter_key = '192.0.2.50')`); err != nil {
			t.Fatal(err)
		}
		if err := second.PasswordLogin(ctx, "192.0.2.50", "still-denied@example.com"); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("source recovered before the hour ended: %v", err)
		}
		if _, err := firstPool.Exec(ctx, `UPDATE identity_limit_counters SET window_start = statement_timestamp() - INTERVAL '61 minutes' WHERE scope = 'source' AND action = 'password-login' AND counter_key = '192.0.2.50' AND window_start = (SELECT MIN(window_start) FROM identity_limit_counters WHERE scope = 'source' AND action = 'password-login' AND counter_key = '192.0.2.50')`); err != nil {
			t.Fatal(err)
		}
		if err := second.PasswordLogin(ctx, "192.0.2.50", "after-window@example.com"); err != nil {
			t.Fatalf("source did not recover after oldest attempt expired: %v", err)
		}
	})

	t.Run("password login email limit is shared and denied requests count nowhere", func(t *testing.T) {
		for i := range 10 {
			limits := first
			if i%2 == 1 {
				limits = second
			}
			if err := limits.PasswordLogin(ctx, fmt.Sprintf("192.0.2.%d", 60+i), "User@EXAMPLE.COM"); err != nil {
				t.Fatalf("email attempt %d: %v", i+1, err)
			}
		}
		if err := second.PasswordLogin(ctx, "192.0.2.70", "User@example.com"); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("email attempt 11 = %v", err)
		}
		var sourceCount, emailCount, exposedKeys int
		if err := firstPool.QueryRow(ctx, `SELECT COALESCE(SUM(count), 0) FROM identity_limit_counters WHERE scope = 'source' AND action = 'password-login' AND counter_key = '192.0.2.70'`).Scan(&sourceCount); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte("User@example.com"))
		key := hex.EncodeToString(digest[:])
		if err := firstPool.QueryRow(ctx, `SELECT COALESCE(SUM(count), 0) FROM identity_limit_counters WHERE scope = 'email' AND action = 'password-login' AND counter_key = $1`, key).Scan(&emailCount); err != nil {
			t.Fatal(err)
		}
		if err := firstPool.QueryRow(ctx, `SELECT COUNT(*) FROM identity_limit_counters WHERE scope = 'email' AND action = 'password-login' AND counter_key LIKE '%@%'`).Scan(&exposedKeys); err != nil {
			t.Fatal(err)
		}
		if sourceCount != 0 || emailCount != 10 || exposedKeys != 0 {
			t.Fatalf("denied email counts = %d source, %d email, %d exposed keys; want 0, 10, 0", sourceCount, emailCount, exposedKeys)
		}
		if _, err := firstPool.Exec(ctx, `UPDATE identity_limit_counters SET window_start = statement_timestamp() - INTERVAL '14 minutes' WHERE scope = 'email' AND action = 'password-login' AND counter_key = $1 AND window_start = (SELECT MIN(window_start) FROM identity_limit_counters WHERE scope = 'email' AND action = 'password-login' AND counter_key = $1)`, key); err != nil {
			t.Fatal(err)
		}
		if err := second.PasswordLogin(ctx, "192.0.2.71", "User@example.com"); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("email recovered before 15 minutes ended: %v", err)
		}
		if _, err := firstPool.Exec(ctx, `UPDATE identity_limit_counters SET window_start = statement_timestamp() - INTERVAL '16 minutes' WHERE scope = 'email' AND action = 'password-login' AND counter_key = $1 AND window_start = (SELECT MIN(window_start) FROM identity_limit_counters WHERE scope = 'email' AND action = 'password-login' AND counter_key = $1)`, key); err != nil {
			t.Fatal(err)
		}
		if err := second.PasswordLogin(ctx, "192.0.2.71", "User@example.com"); err != nil {
			t.Fatalf("email did not recover after oldest attempt expired: %v", err)
		}
	})

	t.Run("concurrent password login attempts share the source limit", func(t *testing.T) {
		var group sync.WaitGroup
		results := make(chan error, 64)
		for i := range 64 {
			group.Add(1)
			go func() {
				defer group.Done()
				limits := first
				if i%2 == 1 {
					limits = second
				}
				results <- limits.PasswordLogin(ctx, "192.0.2.80", fmt.Sprintf("concurrent-source-%d@example.com", i))
			}()
		}
		group.Wait()
		close(results)
		allowed, denied := 0, 0
		for result := range results {
			switch {
			case result == nil:
				allowed++
			case errors.Is(result, app.ErrRateLimited):
				denied++
			default:
				t.Fatalf("unexpected login limit result: %v", result)
			}
		}
		if allowed != 60 || denied != 4 {
			t.Fatalf("concurrent source attempts = %d allowed, %d denied; want 60 and 4", allowed, denied)
		}
	})

	t.Run("concurrent password login attempts share the email limit", func(t *testing.T) {
		var group sync.WaitGroup
		results := make(chan error, 24)
		for i := range 24 {
			group.Add(1)
			go func() {
				defer group.Done()
				limits := first
				if i%2 == 1 {
					limits = second
				}
				results <- limits.PasswordLogin(ctx, fmt.Sprintf("192.0.2.%d", 100+i), "concurrent@example.com")
			}()
		}
		group.Wait()
		close(results)
		allowed, denied := 0, 0
		for result := range results {
			switch {
			case result == nil:
				allowed++
			case errors.Is(result, app.ErrRateLimited):
				denied++
			default:
				t.Fatalf("unexpected login limit result: %v", result)
			}
		}
		if allowed != 10 || denied != 14 {
			t.Fatalf("concurrent login attempts = %d allowed, %d denied; want 10 and 14", allowed, denied)
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
		t.Fatal("unavailable limit repository allowed signup")
	}
	if err := first.PasswordLogin(ctx, "192.0.2.201", "user@example.com"); !errors.Is(err, app.ErrLimitUnavailable) {
		t.Fatalf("unavailable limit repository allowed login: %v", err)
	}
	if err := goose.DownContext(ctx, db, "."); err != nil {
		t.Fatalf("roll back login limit migration: %v", err)
	}
}
