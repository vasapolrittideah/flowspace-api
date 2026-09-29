//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func testVerifyEmail(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Run("email verification consumes only the current code", func(t *testing.T) {
		key := bytes.Repeat([]byte{7}, 32)
		repository := identitypostgres.NewAccountRepository(pool)
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
		service := app.NewEmailVerificationService(repository, limits.WrongCode, limits.AccountWrongCode, key)
		setup := func(t *testing.T, name, purpose string) (inbound.VerifyEmailInput, pgtype.UUID) {
			t.Helper()
			subject := "verify-" + name
			local := "verify-" + name
			email := local + "@example.com"
			queries := identitysqlc.New(pool)
			if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
				Subject: subject, EmailLocal: local, EmailDomain: "example.com", PasswordHash: "$argon2id$test",
			}); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256([]byte(subject))
			session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{
				AccountSubject: subject, RefreshTokenHash: hash[:],
			})
			if err != nil {
				t.Fatal(err)
			}
			code, verifier, _, err := domain.NewChallenge(key, subject, email, domain.CodePurpose(purpose), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			challenge, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
				AccountSubject: subject, Purpose: purpose, EmailLocal: local, EmailDomain: "example.com", CodeVerifier: verifier[:],
			})
			if err != nil {
				t.Fatal(err)
			}
			return inbound.VerifyEmailInput{
				Subject: subject, SessionID: uuid.UUID(session.ID.Bytes).String(),
				Source: "192.0.2.92", Code: code,
			}, challenge.ID
		}
		verified := func(t *testing.T, input inbound.VerifyEmailInput) bool {
			t.Helper()
			var state outbound.AccountState
			if err := repository.WithinVerificationTransaction(ctx, func(tx outbound.VerificationTransaction) error {
				var err error
				state, err = tx.GetActiveAccountForSession(ctx, input.Subject, input.SessionID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			return state.EmailVerified
		}
		valid, validID := setup(t, "valid", "verify-email")
		if err := service.VerifyEmail(ctx, valid); err != nil || !verified(t, valid) {
			t.Fatalf("valid code = %v, verified = %v", err, verified(t, valid))
		}
		if err := service.VerifyEmail(ctx, valid); err != nil {
			t.Fatalf("verified retry = %v", err)
		}
		var consumed sql.NullTime
		if err := pool.QueryRow(ctx, `SELECT consumed_at FROM identity_challenges WHERE id = $1`, validID).Scan(&consumed); err != nil || !consumed.Valid {
			t.Fatalf("consumed challenge = %v, %v", consumed, err)
		}
		for _, name := range []string{"expired", "replaced", "consumed", "claim"} {
			t.Run(name, func(t *testing.T) {
				purpose := "verify-email"
				if name == "claim" {
					purpose = "claim-account"
				}
				input, challengeID := setup(t, name, purpose)
				if name != "claim" {
					if name == "expired" {
						if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at = issued_at - INTERVAL '11 minutes', expires_at = expires_at - INTERVAL '11 minutes' WHERE id = $1`, challengeID); err != nil {
							t.Fatal(err)
						}
					} else {
						column := map[string]string{"replaced": "replaced_at", "consumed": "consumed_at"}[name]
						if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET `+column+` = statement_timestamp() - INTERVAL '1 second' WHERE id = $1`, challengeID); err != nil {
							t.Fatal(err)
						}
					}
				}
				if err := service.VerifyEmail(ctx, input); !errors.Is(err, app.ErrInvalidVerificationCode) || verified(t, input) {
					t.Fatalf("invalid challenge = %v, verified = %v", err, verified(t, input))
				}
			})
		}
		wrong, wrongID := setup(t, "wrong", "verify-email")
		correctCode := wrong.Code
		if wrong.Code == "000000" {
			wrong.Code = "111111"
		} else {
			wrong.Code = "000000"
		}
		for range 5 {
			if err := service.VerifyEmail(ctx, wrong); !errors.Is(err, app.ErrInvalidVerificationCode) {
				t.Fatalf("wrong guess = %v", err)
			}
		}
		var guesses int16
		if err := pool.QueryRow(ctx, `SELECT wrong_guesses FROM identity_challenges WHERE id = $1`, wrongID).Scan(&guesses); err != nil || guesses != 5 || verified(t, wrong) {
			t.Fatalf("wrong guesses = %d, verified = %v, error = %v", guesses, verified(t, wrong), err)
		}
		wrong.Code = correctCode
		if err := service.VerifyEmail(ctx, wrong); !errors.Is(err, app.ErrInvalidVerificationCode) || verified(t, wrong) {
			t.Fatalf("exhausted code = %v, verified = %v", err, verified(t, wrong))
		}
		changedEmail, _ := setup(t, "changed-email", "verify-email")
		if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET email_local = 'verify-new-email' WHERE subject = $1`, changedEmail.Subject); err != nil {
			t.Fatal(err)
		}
		if err := service.VerifyEmail(ctx, changedEmail); !errors.Is(err, app.ErrInvalidVerificationCode) || verified(t, changedEmail) {
			t.Fatalf("changed email = %v, verified = %v", err, verified(t, changedEmail))
		}
		concurrent, concurrentID := setup(t, "concurrent", "verify-email")
		start := make(chan struct{})
		results := make(chan error, 2)
		for range 2 {
			go func() {
				<-start
				results <- service.VerifyEmail(ctx, concurrent)
			}()
		}
		close(start)
		for range 2 {
			err := <-results
			if err != nil {
				t.Fatalf("concurrent verification = %v", err)
			}
		}
		var consumedCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE id = $1 AND consumed_at IS NOT NULL`, concurrentID).Scan(&consumedCount); err != nil || consumedCount != 1 || !verified(t, concurrent) {
			t.Fatalf("concurrent consumption = %d, verified = %v, error = %v", consumedCount, verified(t, concurrent), err)
		}
	})
}
