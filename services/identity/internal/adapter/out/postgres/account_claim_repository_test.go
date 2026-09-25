//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type accountClaimFixture struct {
	email, subject, claimCode, verifyCode string
	sessionID, claimID, verifyID          pgtype.UUID
}

func seedAccountClaim(t *testing.T, ctx context.Context, pool *pgxpool.Pool, local string, key []byte) accountClaimFixture {
	t.Helper()
	fixture := accountClaimFixture{email: local + "@example.com", subject: "claim-" + local}
	queries := identitysqlc.New(pool)
	if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
		Subject: fixture.subject, EmailLocal: local, EmailDomain: "example.com", PasswordHash: "$argon2id$old",
	}); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(fixture.subject))
	session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: fixture.subject, RefreshTokenHash: hash[:]})
	if err != nil {
		t.Fatal(err)
	}
	fixture.sessionID = session.ID
	for _, purpose := range []domain.CodePurpose{domain.PurposeClaimAccount, domain.PurposeVerifyEmail} {
		code, verifier, _, err := domain.NewChallenge(key, fixture.subject, fixture.email, purpose, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		challenge, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
			AccountSubject: fixture.subject, Purpose: string(purpose), EmailLocal: local,
			EmailDomain: "example.com", CodeVerifier: verifier[:],
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := queries.StoreChallengeDelivery(ctx, identitysqlc.StoreChallengeDeliveryParams{
			ChallengeID: challenge.ID, KeyVersion: 1, Nonce: bytes.Repeat([]byte{2}, 12), Ciphertext: bytes.Repeat([]byte{3}, 17),
		}); err != nil {
			t.Fatal(err)
		}
		if purpose == domain.PurposeClaimAccount {
			fixture.claimCode, fixture.claimID = code, challenge.ID
		} else {
			fixture.verifyCode, fixture.verifyID = code, challenge.ID
		}
	}
	return fixture
}

func (f accountClaimFixture) input() inbound.ClaimAccountInput {
	return inbound.ClaimAccountInput{Email: f.email, Code: f.claimCode, NewPassword: "correct horse battery staple", Source: "192.0.2.130"}
}

func testAccountClaimRepository(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	key := bytes.Repeat([]byte{12}, 32)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(privateKey, "claim-test", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	repository := identitypostgres.NewAccountRepository(pool)
	limits := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
	compromised := func(context.Context, string) (bool, error) { return false, nil }
	service := app.NewAccountClaimService(repository, signer, limits.WrongCode, limits.AccountWrongCode, compromised, key)
	verification := app.NewVerificationCodeService(repository, nil, nil, limits.WrongCode, limits.AccountWrongCode, key)

	t.Run("missing and verified accounts have the same claim error", func(t *testing.T) {
		verified := seedAccountClaim(t, ctx, pool, "ClaimVerified", key)
		if _, err := identitysqlc.New(pool).MarkEmailVerified(ctx, verified.subject); err != nil {
			t.Fatal(err)
		}
		for _, input := range []inbound.ClaimAccountInput{
			{Email: "ClaimMissing@example.com", Code: verified.claimCode, NewPassword: "correct horse battery staple", Source: "192.0.2.130"},
			verified.input(),
		} {
			result, err := service.ClaimUnverifiedAccount(ctx, input)
			if !errors.Is(err, app.ErrInvalidClaimCode) || result.Subject != "" || result.RefreshToken != "" {
				t.Fatalf("ineligible claim = %+v, error = %v", result, err)
			}
		}
	})

	t.Run("success retires the old subject and cannot replay tokens", func(t *testing.T) {
		old := seedAccountClaim(t, ctx, pool, "ClaimSuccess", key)
		result, err := service.ClaimUnverifiedAccount(ctx, old.input())
		if err != nil || result.Subject == "" || result.Subject == old.subject || result.AccessToken == "" || result.RefreshToken == "" {
			t.Fatalf("claim result = %+v, error = %v", result, err)
		}
		var retired, verified, consumed, verificationReplaced sql.NullTime
		var passwordHash string
		if err := pool.QueryRow(ctx, `SELECT retired_at FROM identity_accounts WHERE subject = $1`, old.subject).Scan(&retired); err != nil || !retired.Valid {
			t.Fatalf("old account retirement = %v, error = %v", retired, err)
		}
		if err := pool.QueryRow(ctx, `SELECT email_verified_at, password_hash FROM identity_accounts WHERE subject = $1`, result.Subject).Scan(&verified, &passwordHash); err != nil || !verified.Valid || passwordHash == "$argon2id$old" {
			t.Fatalf("new account verified = %v, hash replaced = %t, error = %v", verified, passwordHash != "$argon2id$old", err)
		}
		if err := pool.QueryRow(ctx, `SELECT consumed_at FROM identity_challenges WHERE id = $1`, old.claimID).Scan(&consumed); err != nil || !consumed.Valid {
			t.Fatalf("claim code consumed = %v, error = %v", consumed, err)
		}
		if err := pool.QueryRow(ctx, `SELECT replaced_at FROM identity_challenges WHERE id = $1`, old.verifyID).Scan(&verificationReplaced); err != nil || !verificationReplaced.Valid {
			t.Fatalf("verification code revoked = %v, error = %v", verificationReplaced, err)
		}
		var active, revokedSessions, oldDeliveries int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE email_local = 'ClaimSuccess' AND retired_at IS NULL`).Scan(&active); err != nil || active != 1 {
			t.Fatalf("active email owners = %d, error = %v", active, err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_sessions WHERE account_subject = $1 AND revoked_at IS NOT NULL`, old.subject).Scan(&revokedSessions); err != nil || revokedSessions != 1 {
			t.Fatalf("revoked old sessions = %d, error = %v", revokedSessions, err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenge_deliveries AS delivery JOIN identity_challenges AS challenge ON challenge.id = delivery.challenge_id WHERE challenge.account_subject = $1`, old.subject).Scan(&oldDeliveries); err != nil || oldDeliveries != 0 {
			t.Fatalf("old delivery material = %d, error = %v", oldDeliveries, err)
		}
		if err := repository.WithinVerificationTransaction(ctx, func(tx outbound.VerificationTransaction) error {
			_, err := tx.GetActiveAccountForSession(ctx, old.subject, uuid.UUID(old.sessionID.Bytes).String())
			return err
		}); !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatalf("old session remains active: %v", err)
		}
		var newSession pgtype.UUID
		if err := pool.QueryRow(ctx, `SELECT id FROM identity_sessions WHERE account_subject = $1 AND revoked_at IS NULL`, result.Subject).Scan(&newSession); err != nil {
			t.Fatal(err)
		}
		var newSessionCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_sessions WHERE account_subject = $1`, result.Subject).Scan(&newSessionCount); err != nil || newSessionCount != 1 {
			t.Fatalf("new sessions = %d, error = %v", newSessionCount, err)
		}
		if err := repository.WithinVerificationTransaction(ctx, func(tx outbound.VerificationTransaction) error {
			state, err := tx.GetActiveAccountForSession(ctx, result.Subject, uuid.UUID(newSession.Bytes).String())
			if err == nil && !state.EmailVerified {
				t.Fatal("new session did not see verified email")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		replayed, err := service.ClaimUnverifiedAccount(ctx, old.input())
		if !errors.Is(err, app.ErrInvalidClaimCode) || replayed.AccessToken != "" || replayed.RefreshToken != "" {
			t.Fatalf("replayed claim = %+v, error = %v", replayed, err)
		}
		if err := verification.VerifyEmail(ctx, inbound.VerifyEmailInput{
			Subject: old.subject, SessionID: uuid.UUID(old.sessionID.Bytes).String(), Source: "192.0.2.131", Code: old.verifyCode,
		}); !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatalf("retired account verification = %v", err)
		}
	})

	t.Run("wrong purpose expiry and guess exhaustion keep the old account", func(t *testing.T) {
		for _, name := range []string{"purpose", "expired", "guesses"} {
			t.Run(name, func(t *testing.T) {
				old := seedAccountClaim(t, ctx, pool, "ClaimInvalid"+name, key)
				input := old.input()
				switch name {
				case "purpose":
					if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET replaced_at = statement_timestamp() WHERE id = $1`, old.claimID); err != nil {
						t.Fatal(err)
					}
					input.Code = old.verifyCode
				case "expired":
					if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at = issued_at - INTERVAL '11 minutes', expires_at = expires_at - INTERVAL '11 minutes' WHERE id = $1`, old.claimID); err != nil {
						t.Fatal(err)
					}
				case "guesses":
					if input.Code == "000000" {
						input.Code = "111111"
					} else {
						input.Code = "000000"
					}
					for range 5 {
						if _, err := service.ClaimUnverifiedAccount(ctx, input); !errors.Is(err, app.ErrInvalidClaimCode) {
							t.Fatalf("wrong guess = %v", err)
						}
					}
					input.Code = old.claimCode
				}
				if _, err := service.ClaimUnverifiedAccount(ctx, input); !errors.Is(err, app.ErrInvalidClaimCode) {
					t.Fatalf("%s claim = %v", name, err)
				}
				var retired sql.NullTime
				if err := pool.QueryRow(ctx, `SELECT retired_at FROM identity_accounts WHERE subject = $1`, old.subject).Scan(&retired); err != nil || retired.Valid {
					t.Fatalf("old account retired = %v, error = %v", retired, err)
				}
			})
		}
	})

	t.Run("signing failure rolls back retirement and code consumption", func(t *testing.T) {
		old := seedAccountClaim(t, ctx, pool, "ClaimRollback", key)
		failed := app.NewAccountClaimService(repository, failingTokenSigner{}, limits.WrongCode, limits.AccountWrongCode, compromised, key)
		result, err := failed.ClaimUnverifiedAccount(ctx, old.input())
		if !errors.Is(err, app.ErrClaimUnavailable) || result.RefreshToken != "" {
			t.Fatalf("failed claim = %+v, error = %v", result, err)
		}
		var retired, consumed sql.NullTime
		if err := pool.QueryRow(ctx, `SELECT retired_at FROM identity_accounts WHERE subject = $1`, old.subject).Scan(&retired); err != nil || retired.Valid {
			t.Fatalf("rollback retired old account = %v, error = %v", retired, err)
		}
		if err := pool.QueryRow(ctx, `SELECT consumed_at FROM identity_challenges WHERE id = $1`, old.claimID).Scan(&consumed); err != nil || consumed.Valid {
			t.Fatalf("rollback consumed claim code = %v, error = %v", consumed, err)
		}
		if result, err := service.ClaimUnverifiedAccount(ctx, old.input()); err != nil || result.Subject == "" {
			t.Fatalf("claim after rollback = %+v, error = %v", result, err)
		}
	})

	t.Run("competing claims and verification commit one transition", func(t *testing.T) {
		old := seedAccountClaim(t, ctx, pool, "ClaimRace", key)
		start := make(chan struct{})
		claimResult := make(chan error, 2)
		for range 2 {
			go func() {
				<-start
				_, err := service.ClaimUnverifiedAccount(ctx, old.input())
				claimResult <- err
			}()
		}
		close(start)
		wins, rejects := 0, 0
		for range 2 {
			switch err := <-claimResult; {
			case err == nil:
				wins++
			case errors.Is(err, app.ErrInvalidClaimCode):
				rejects++
			default:
				t.Fatalf("concurrent claim = %v", err)
			}
		}
		if wins != 1 || rejects != 1 {
			t.Fatalf("claim wins = %d, rejects = %d", wins, rejects)
		}
		old = seedAccountClaim(t, ctx, pool, "ClaimVerifyRace", key)
		start = make(chan struct{})
		claimResult = make(chan error, 1)
		verifyResult := make(chan error, 1)
		go func() {
			<-start
			_, err := service.ClaimUnverifiedAccount(ctx, old.input())
			claimResult <- err
		}()
		go func() {
			<-start
			verifyResult <- verification.VerifyEmail(ctx, inbound.VerifyEmailInput{
				Subject: old.subject, SessionID: uuid.UUID(old.sessionID.Bytes).String(), Source: "192.0.2.132", Code: old.verifyCode,
			})
		}()
		close(start)
		claimErr, verifyErr := <-claimResult, <-verifyResult
		if !(claimErr == nil && errors.Is(verifyErr, outbound.ErrUnauthenticated) ||
			errors.Is(claimErr, app.ErrInvalidClaimCode) && verifyErr == nil) {
			t.Fatalf("claim/verification race = claim %v, verify %v", claimErr, verifyErr)
		}
		var active, verified int
		if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE email_verified_at IS NOT NULL) FROM identity_accounts WHERE email_local = 'ClaimVerifyRace' AND retired_at IS NULL`).Scan(&active, &verified); err != nil || active != 1 || verified != 1 {
			t.Fatalf("race left %d active, %d verified, error %v", active, verified, err)
		}
	})
}
