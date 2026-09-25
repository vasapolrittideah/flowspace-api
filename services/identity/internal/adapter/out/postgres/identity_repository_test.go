//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
)

type failingTokenSigner struct{}

func (failingTokenSigner) Sign(app.AccessTokenClaims) (string, error) {
	return "", errors.New("signing failed")
}

func TestIdentityRepository(t *testing.T) {
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
	t.Cleanup(func() { _ = db.Close() })
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.Files)
	if err := goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	t.Run("active email uses case-sensitive local and folded domain", func(t *testing.T) {
		insertAccount := func(subject, local, domain string) error {
			_, err := pool.Exec(ctx, `INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash) VALUES ($1, $2, $3, '$argon2id$test')`, subject, local, domain)
			return err
		}
		if err := insertAccount("subject-1", "User", "example.com"); err != nil {
			t.Fatal(err)
		}
		if err := insertAccount("subject-2", "User", "EXAMPLE.COM"); err == nil {
			t.Fatal("uppercase domain was accepted")
		}
		if err := insertAccount("subject-3", "User", "example.com"); err == nil {
			t.Fatal("duplicate active email was accepted")
		}
		if err := insertAccount("subject-4", "user", "example.com"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("concurrent signup has one active email owner", func(t *testing.T) {
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, subject := range []string{"racer-1", "racer-2"} {
			go func() {
				<-start
				_, err := pool.Exec(ctx, `INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash) VALUES ($1, 'Race', 'example.com', '$argon2id$test')`, subject)
				results <- err
			}()
		}
		close(start)
		wins, conflicts := 0, 0
		for range 2 {
			err := <-results
			if err == nil {
				wins++
				continue
			}
			var postgresError *pgconn.PgError
			if !errors.As(err, &postgresError) || postgresError.Code != "23505" {
				t.Fatalf("unexpected concurrent signup error: %v", err)
			}
			conflicts++
		}
		if wins != 1 || conflicts != 1 {
			t.Fatalf("concurrent signup wins = %d, conflicts = %d", wins, conflicts)
		}
	})

	t.Run("signup records roll back together", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		queries := identitysqlc.New(tx)
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "rollback", EmailLocal: "rollback", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{
			AccountSubject: "rollback", RefreshTokenHash: bytes.Repeat([]byte{5}, 32),
		}); err != nil {
			t.Fatal(err)
		}
		challenge, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
			AccountSubject: "rollback", Purpose: "verify-email", EmailLocal: "rollback",
			EmailDomain: "example.com", CodeVerifier: bytes.Repeat([]byte{6}, 32),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := queries.StoreChallengeDelivery(ctx, identitysqlc.StoreChallengeDeliveryParams{
			ChallengeID: challenge.ID, KeyVersion: 1,
			Nonce: bytes.Repeat([]byte{7}, 12), Ciphertext: bytes.Repeat([]byte{8}, 17),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := queries.CreateOutboxEvent(ctx, challenge.ID); err != nil {
			t.Fatal(err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		for _, check := range []struct{ name, query string }{
			{"account", `SELECT count(*) FROM identity_accounts WHERE subject = 'rollback'`},
			{"session", `SELECT count(*) FROM identity_sessions WHERE account_subject = 'rollback'`},
			{"challenge", `SELECT count(*) FROM identity_challenges WHERE account_subject = 'rollback'`},
			{"delivery", `SELECT count(*) FROM identity_challenge_deliveries`},
			{"outbox event", `SELECT count(*) FROM identity_outbox_events`},
		} {
			var count int
			if err := pool.QueryRow(ctx, check.query).Scan(&count); err != nil || count != 0 {
				t.Fatalf("%s has %d rows after rollback: %v", check.name, count, err)
			}
		}
	})

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

	t.Run("typed claim retirement succeeds once", func(t *testing.T) {
		queries := identitysqlc.New(pool)
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "claim-old", EmailLocal: "claim", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		account, err := queries.GetActiveAccountByEmailForUpdate(ctx, identitysqlc.GetActiveAccountByEmailForUpdateParams{
			EmailLocal: "claim", EmailDomain: "example.com",
		})
		if err != nil || account.Subject != "claim-old" {
			t.Fatalf("account locked by email = %+v, %v", account, err)
		}
		for attempt, want := range []int64{1, 0} {
			count, err := queries.RetireUnverifiedAccount(ctx, "claim-old")
			if err != nil || count != want {
				t.Fatalf("retirement %d changed %d rows, %v", attempt+1, count, err)
			}
		}
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "claim-new", EmailLocal: "claim", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		changed, err := queries.MarkEmailVerified(ctx, "claim-new")
		if err != nil || changed != 1 {
			t.Fatalf("verified %d accounts, %v", changed, err)
		}
		if changed, err := queries.RetireUnverifiedAccount(ctx, "claim-new"); err != nil || changed != 0 {
			t.Fatalf("verified account retired %d times, %v", changed, err)
		}
	})

	t.Run("typed signup records and one-use challenge", func(t *testing.T) {
		queries := identitysqlc.New(pool)
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "challenge-subject", EmailLocal: "Challenge", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{
			AccountSubject: "challenge-subject", RefreshTokenHash: bytes.Repeat([]byte{1}, 32),
		}); err != nil {
			t.Fatal(err)
		}
		account, err := queries.GetActiveAccountForUpdate(ctx, "challenge-subject")
		if err != nil || account.Subject != "challenge-subject" {
			t.Fatalf("account locked by subject = %+v, %v", account, err)
		}
		newChallenge := identitysqlc.CreateChallengeParams{
			AccountSubject: "challenge-subject", Purpose: "verify-email",
			EmailLocal: "Challenge", EmailDomain: "example.com", CodeVerifier: bytes.Repeat([]byte{2}, 32),
		}
		first, err := queries.CreateChallenge(ctx, newChallenge)
		if err != nil {
			t.Fatal(err)
		}
		current, err := queries.GetCurrentChallengeForUpdate(ctx, identitysqlc.GetCurrentChallengeForUpdateParams{
			AccountSubject: "challenge-subject", Purpose: "verify-email",
		})
		if err != nil || current.ID != first.ID || !bytes.Equal(current.CodeVerifier, newChallenge.CodeVerifier) {
			t.Fatalf("current challenge = %+v, %v", current, err)
		}
		if guesses, err := queries.IncrementChallengeWrongGuess(ctx, first.ID); err != nil || guesses != 1 {
			t.Fatalf("wrong guesses = %d, %v", guesses, err)
		}
		if _, err := queries.CreateChallenge(ctx, newChallenge); err == nil {
			t.Fatal("two current verification challenges were accepted")
		}
		if err := queries.StoreChallengeDelivery(ctx, identitysqlc.StoreChallengeDeliveryParams{
			ChallengeID: first.ID, KeyVersion: 1, Nonce: bytes.Repeat([]byte{3}, 12), Ciphertext: bytes.Repeat([]byte{4}, 17),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := queries.CreateOutboxEvent(ctx, first.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := queries.CreateOutboxEvent(ctx, first.ID); err == nil {
			t.Fatal("duplicate outbox event for one challenge was accepted")
		}
		if changed, err := queries.DeleteChallengeDelivery(ctx, first.ID); err != nil || changed != 1 {
			t.Fatalf("deleted %d delivery payloads, %v", changed, err)
		}
		changed, err := queries.ReplaceCurrentChallenge(ctx, identitysqlc.ReplaceCurrentChallengeParams{
			AccountSubject: "challenge-subject", Purpose: "verify-email",
		})
		if err != nil || changed != 1 {
			t.Fatalf("replaced %d challenges, %v", changed, err)
		}
		second, err := queries.CreateChallenge(ctx, newChallenge)
		if err != nil {
			t.Fatal(err)
		}
		for attempt, want := range []int64{1, 0} {
			changed, err := queries.ConsumeCurrentChallenge(ctx, second.ID)
			if err != nil || changed != want {
				t.Fatalf("consume attempt %d changed %d rows, %v", attempt+1, changed, err)
			}
		}
		newChallenge.Purpose = "claim-account"
		claim, err := queries.CreateChallenge(ctx, newChallenge)
		if err != nil {
			t.Fatal(err)
		}
		changed, err = queries.RevokeAccountChallenges(ctx, "challenge-subject")
		if err != nil || changed != 1 {
			t.Fatalf("revoked %d current challenges, %v", changed, err)
		}
		if changed, err := queries.ConsumeCurrentChallenge(ctx, claim.ID); err != nil || changed != 0 {
			t.Fatalf("revoked claim challenge consumed %d times, %v", changed, err)
		}
		for want := int32(1); want <= 2; want++ {
			count, err := queries.IncrementLimitCounter(ctx, identitysqlc.IncrementLimitCounterParams{
				Scope: "account", CounterKey: "challenge-subject", Action: "code-request",
				WindowStart: pgtype.Timestamptz{Time: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC), Valid: true},
			})
			if err != nil || count != want {
				t.Fatalf("limit count = %d, %v; want %d", count, err, want)
			}
		}
		claimed, err := queries.ClaimOutboxEvent(ctx, pgtype.Text{String: "worker-1", Valid: true})
		if err != nil || claimed.ChallengeID != first.ID {
			t.Fatalf("outbox claim = %+v, %v", claimed, err)
		}
		if _, err := queries.ClaimOutboxEvent(ctx, pgtype.Text{String: "worker-2", Valid: true}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("another worker claimed leased event: %v", err)
		}
		changed, err = queries.ReleaseOutboxClaim(ctx, identitysqlc.ReleaseOutboxClaimParams{
			ID: claimed.ID, ClaimOwner: pgtype.Text{String: "worker-1", Valid: true},
			NextAttemptAt: pgtype.Timestamptz{Time: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
		})
		if err != nil || changed != 1 {
			t.Fatalf("released %d outbox claims, %v", changed, err)
		}
		claimed, err = queries.ClaimOutboxEvent(ctx, pgtype.Text{String: "worker-2", Valid: true})
		if err != nil || claimed.ChallengeID != first.ID {
			t.Fatalf("retry outbox claim = %+v, %v", claimed, err)
		}
		changed, err = queries.MarkOutboxPublished(ctx, identitysqlc.MarkOutboxPublishedParams{
			ID: claimed.ID, ClaimOwner: pgtype.Text{String: "worker-2", Valid: true},
		})
		if err != nil || changed != 1 {
			t.Fatalf("published %d outbox events, %v", changed, err)
		}
		if _, err := queries.ClaimOutboxEvent(ctx, pgtype.Text{String: "worker-3", Valid: true}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("published event was claimed again: %v", err)
		}
		if changed, err := queries.RevokeAccountSessions(ctx, "challenge-subject"); err != nil || changed != 1 {
			t.Fatalf("revoked %d sessions, %v", changed, err)
		}
	})
}
