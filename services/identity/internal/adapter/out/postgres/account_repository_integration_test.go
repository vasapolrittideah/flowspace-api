//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net"
	"slices"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/grpc/peer"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type failingTokenSigner struct{}

func (failingTokenSigner) Sign(outbound.AccessTokenClaims) (string, error) {
	return "", errors.New("signing failed")
}

type failingDeliveryProtector struct{}

func (failingDeliveryProtector) Protect(_, _, _, _, _ string) (outbound.DeliveryMaterial, error) {
	return outbound.DeliveryMaterial{}, errors.New("encryption failed")
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
	t.Run("refresh rotation and replay", func(t *testing.T) { testRefreshSessionRepository(t, pool) })
	t.Run("provider-only refresh", func(t *testing.T) { testProviderOnlyRefresh(t, pool) })
	t.Run("current session logout", func(t *testing.T) { testCurrentSessionLogout(t, pool) })
	t.Run("all session logout", func(t *testing.T) { testAllSessionLogout(t, pool) })

	testSessionCheckRepository(t, ctx, pool, dsn)
	testActiveEmailUniqueness(t, ctx, pool)
	testConcurrentSignup(t, ctx, pool)
	testSignupRollback(t, ctx, pool)
	testSessionIssuanceRollback(t, ctx, pool)
	testClaimRetirement(t, ctx, pool)
	testSignupChallengeRecords(t, ctx, pool)
	testSignupTransaction(t, ctx, pool)
	testSignupSigningFailure(t, ctx, pool)
	testVerificationCodeResend(t, ctx, pool)
	testVerifyEmail(t, ctx, pool)
	testClaimCode(t, ctx, pool)
	testRequestPasswordResetCode(t, ctx, pool)
	testProviderAttemptRepository(t, ctx, pool)
	t.Run("provider sessions claim one handoff for a linked account", func(t *testing.T) {
		testProviderSessionRepository(t, ctx, pool, dsn)
	})

	t.Run("account claims replace one unverified identity", func(t *testing.T) {
		testAccountClaimRepository(t, pool)
	})
	t.Run("password login commits one session and hides missing accounts", func(t *testing.T) {
		testPasswordLoginRepository(t, pool)
	})
	t.Run("password reset consumes proof and revokes sessions atomically", func(t *testing.T) {
		testPasswordResetRepository(t, pool)
	})
	t.Run("invalid reset codes keep practical timing across account states", func(t *testing.T) {
		testPasswordResetInvalidCodeTiming(t, pool)
	})
	t.Run("password recovery treats provider accounts by their password credential", func(t *testing.T) {
		testPasswordRecoveryForProviderAccounts(t, pool)
	})
}

func testActiveEmailUniqueness(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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
}

func testConcurrentSignup(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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
}

func testSignupRollback(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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
}

func testSignupChallengeRecords(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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

func testSignupTransaction(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Run("signup commits account session code and outbox without broker", func(t *testing.T) {
		_, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := token.NewSigner(privateKey, "signup-test", "urn:flowspace:identity:local", "flowspace-api")
		if err != nil {
			t.Fatal(err)
		}
		protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{3}, 32), 1)
		if err != nil {
			t.Fatal(err)
		}
		verifierKey := bytes.Repeat([]byte{4}, 32)
		service := app.NewSignupService(identitypostgres.NewAccountRepository(pool), signer, protector,
			func(context.Context, string) error { return nil },
			func(context.Context, string) (bool, error) { return false, nil }, verifierKey)
		request := inbound.CreateAccountInput{Email: "Signup@EXAMPLE.COM", Password: "correct horse battery staple", Source: "192.0.2.1"}
		result, err := service.CreateAccount(ctx, request)
		if err != nil || result.Subject == "" || result.AccessToken == "" || result.RefreshToken == "" {
			t.Fatalf("signup result = %+v, error = %v", result, err)
		}
		var challengeID pgtype.UUID
		var verifier, nonce, ciphertext, refreshHash []byte
		var keyVersion int32
		var expires time.Time
		var verified sql.NullTime
		err = pool.QueryRow(ctx, `SELECT challenge.id, challenge.code_verifier, challenge.expires_at, delivery.key_version, delivery.nonce, delivery.ciphertext, session.refresh_token_hash, account.email_verified_at
			FROM identity_accounts account
			JOIN identity_sessions session ON session.account_subject = account.subject
			JOIN identity_challenges challenge ON challenge.account_subject = account.subject
			JOIN identity_challenge_deliveries delivery ON delivery.challenge_id = challenge.id
			JOIN identity_outbox_events event ON event.challenge_id = challenge.id
			WHERE account.subject = $1 AND account.email_local = 'Signup' AND account.email_domain = 'example.com'
			AND challenge.purpose = 'verify-email' AND event.published_at IS NULL`, result.Subject).Scan(
			&challengeID, &verifier, &expires, &keyVersion, &nonce, &ciphertext, &refreshHash, &verified)
		if err != nil || verified.Valid || len(verifier) != 32 || len(refreshHash) != 32 {
			t.Fatalf("committed signup records: verified = %v, error = %v", verified, err)
		}
		if bytes.Equal(refreshHash, []byte(result.RefreshToken)) {
			t.Fatal("refresh token stored in plaintext")
		}
		email, code, err := protector.Open(uuid.UUID(challengeID.Bytes).String(), "verify-email", result.Subject,
			outbound.DeliveryMaterial{KeyVersion: keyVersion, Nonce: nonce, Ciphertext: ciphertext})
		var expected [32]byte
		copy(expected[:], verifier)
		if err != nil || email != "Signup@example.com" || !domain.VerifyChallenge(verifierKey, result.Subject, email, domain.PurposeVerifyEmail, code, expected, expires, time.Now()) {
			t.Fatalf("stored challenge cannot deliver current code: %v", err)
		}
		outbox := identitypostgres.NewOutboxRepository(pool)
		deliveryRequest, ok, err := outbox.Claim(ctx, "relay-1")
		if err != nil || !ok || deliveryRequest.ChallengeID != uuid.UUID(challengeID.Bytes).String() || deliveryRequest.Purpose != "verify-email" {
			t.Fatalf("claimed delivery request = %+v, %v", deliveryRequest, err)
		}
		if err := outbox.Release(ctx, deliveryRequest.ID, "relay-1", time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		retriedRequest, ok, err := outbox.Claim(ctx, "relay-2")
		if err != nil || !ok || retriedRequest != deliveryRequest {
			t.Fatalf("retried delivery request = %+v, %v", retriedRequest, err)
		}
		if err := outbox.MarkPublished(ctx, retriedRequest.ID, "relay-2"); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := outbox.Claim(ctx, "relay-3"); err != nil || ok {
			t.Fatalf("published event was claimable: %v, %v", ok, err)
		}
		if err := outbox.MarkPublished(ctx, retriedRequest.ID, "relay-2"); !errors.Is(err, identitypostgres.ErrOutboxClaimLost) {
			t.Fatalf("stale publication mark = %v", err)
		}
		if err := outbox.Release(ctx, retriedRequest.ID, "relay-2", time.Now()); !errors.Is(err, identitypostgres.ErrOutboxClaimLost) {
			t.Fatalf("stale claim release = %v", err)
		}
		if err := outbox.MarkPublished(ctx, "invalid-id", "relay-2"); err == nil {
			t.Fatal("invalid event ID was marked published")
		}
		if err := outbox.Release(ctx, "invalid-id", "relay-2", time.Now()); err == nil {
			t.Fatal("invalid event ID was released")
		}
		if retry, err := service.CreateAccount(ctx, request); !errors.Is(err, outbound.ErrAccountExists) || retry.AccessToken != "" || retry.RefreshToken != "" {
			t.Fatalf("lost-response retry = %+v, %v", retry, err)
		}
		login := app.NewPasswordLoginService(identitypostgres.NewAccountRepository(pool), signer, func(context.Context, string, string) error { return nil })
		recovered, err := login.CreatePasswordSession(ctx, inbound.CreatePasswordSessionInput{
			Email: request.Email, Password: request.Password, Source: "192.0.2.2",
		})
		if err != nil || recovered.Subject != result.Subject || recovered.EmailVerified || recovered.AccessToken == "" || recovered.RefreshToken == "" || recovered.AccessToken == result.AccessToken || recovered.RefreshToken == result.RefreshToken {
			t.Fatalf("login after lost signup response = %+v, %v", recovered, err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		for range 2 {
			go func() {
				<-start
				_, err := service.CreateAccount(ctx, inbound.CreateAccountInput{
					Email: "concurrent-signup@example.com", Password: "correct horse battery staple", Source: "192.0.2.3",
				})
				results <- err
			}()
		}
		close(start)
		wins, duplicates := 0, 0
		for range 2 {
			switch err := <-results; {
			case err == nil:
				wins++
			case errors.Is(err, outbound.ErrAccountExists):
				duplicates++
			default:
				t.Fatalf("concurrent signup error: %v", err)
			}
		}
		if wins != 1 || duplicates != 1 {
			t.Fatalf("concurrent signup wins = %d, duplicates = %d", wins, duplicates)
		}
		lateFailure := app.NewSignupService(identitypostgres.NewAccountRepository(pool), signer, failingDeliveryProtector{},
			func(context.Context, string) error { return nil },
			func(context.Context, string) (bool, error) { return false, nil }, verifierKey)
		failed, err := lateFailure.CreateAccount(ctx, inbound.CreateAccountInput{
			Email: "late-failure@example.com", Password: "correct horse battery staple", Source: "192.0.2.4",
		})
		if err == nil || failed.AccessToken != "" || failed.RefreshToken != "" {
			t.Fatalf("late failure returned tokens: %+v, %v", failed, err)
		}
		var accounts, challenges int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE email_local = 'late-failure'`).Scan(&accounts); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE email_local = 'late-failure'`).Scan(&challenges); err != nil {
			t.Fatal(err)
		}
		if accounts != 0 || challenges != 0 {
			t.Fatalf("late failure committed %d accounts and %d challenges", accounts, challenges)
		}
	})
}

func testSignupSigningFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Run("signing failure rolls back every signup record", func(t *testing.T) {
		protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{3}, 32), 1)
		if err != nil {
			t.Fatal(err)
		}
		service := app.NewSignupService(identitypostgres.NewAccountRepository(pool), failingTokenSigner{}, protector,
			func(context.Context, string) error { return nil },
			func(context.Context, string) (bool, error) { return false, nil }, bytes.Repeat([]byte{4}, 32))
		result, err := service.CreateAccount(ctx, inbound.CreateAccountInput{Email: "failure@example.com", Password: "correct horse battery staple", Source: "192.0.2.2"})
		if err == nil || result.AccessToken != "" || result.RefreshToken != "" {
			t.Fatalf("failed signup = %+v, %v", result, err)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE email_local = 'failure'`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rolled back accounts = %d, %v", count, err)
		}
	})
}

func testPasswordLoginRepository(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	hash, err := domain.HashPassword(ctx, "correct horse battery staple", func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	queries := identitysqlc.New(pool)
	if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{Subject: "login-subject", EmailLocal: "Login", EmailDomain: "example.com", PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(privateKey, "login-key", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	repo := identitypostgres.NewAccountRepository(pool)
	service := app.NewPasswordLoginService(repo, signer, func(context.Context, string, string) error { return nil })
	input := inbound.CreatePasswordSessionInput{Email: "Login@EXAMPLE.COM", Password: "correct horse battery staple", Source: "192.0.2.1"}
	result, err := service.CreatePasswordSession(ctx, input)
	if err != nil || result.Subject != "login-subject" || result.EmailVerified || result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatalf("unverified login failed: %v", err)
	}
	verifier, err := token.NewVerifier(map[string]ed25519.PublicKey{"login-key": publicKey}, "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := verifier.Verify(result.AccessToken)
	if err != nil || identity.Subject != "login-subject" || identity.SessionID == "" {
		t.Fatalf("invalid access token: %v", err)
	}
	verified, err := app.NewSessionCheckService(identitypostgres.NewSessionRepository(pool)).CheckSession(ctx, inbound.CheckSessionInput{Subject: identity.Subject, SessionID: identity.SessionID})
	if err != nil || verified {
		t.Fatalf("new session check = %t, error = %v", verified, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_sessions WHERE account_subject = $1`, "login-subject").Scan(&count); err != nil || count != 1 {
		t.Fatalf("session count = %d, error = %v", count, err)
	}
	if _, err := queries.MarkEmailVerified(ctx, "login-subject"); err != nil {
		t.Fatal(err)
	}
	firstToken := result.RefreshToken
	result, err = service.CreatePasswordSession(ctx, input)
	if err != nil || !result.EmailVerified || result.RefreshToken == firstToken {
		t.Fatalf("verified login failed: %v", err)
	}
	if verified, err := app.NewSessionCheckService(identitypostgres.NewSessionRepository(pool)).CheckSession(ctx,
		inbound.CheckSessionInput{Subject: identity.Subject, SessionID: identity.SessionID}); err != nil || !verified {
		t.Fatalf("first session after repeated login = %t, %v", verified, err)
	}
	for _, invalid := range []inbound.CreatePasswordSessionInput{
		{Email: "missing@example.com", Password: input.Password, Source: input.Source},
		{Email: input.Email, Password: "wrong-password", Source: input.Source},
	} {
		result, err := service.CreatePasswordSession(ctx, invalid)
		if !errors.Is(err, app.ErrInvalidCredentials) || result.AccessToken != "" {
			t.Fatalf("invalid login returned %v", err)
		}
	}
	var missingTimes, wrongTimes []time.Duration
	for range 8 {
		for _, sample := range []struct {
			input inbound.CreatePasswordSessionInput
			times *[]time.Duration
		}{
			{inbound.CreatePasswordSessionInput{Email: "missing@example.com", Password: "wrong-password", Source: input.Source}, &missingTimes},
			{inbound.CreatePasswordSessionInput{Email: input.Email, Password: "wrong-password", Source: input.Source}, &wrongTimes},
		} {
			started := time.Now()
			if _, err := service.CreatePasswordSession(ctx, sample.input); !errors.Is(err, app.ErrInvalidCredentials) {
				t.Fatalf("invalid login = %v", err)
			}
			*sample.times = append(*sample.times, time.Since(started))
		}
	}
	sort.Slice(missingTimes, func(i, j int) bool { return missingTimes[i] < missingTimes[j] })
	sort.Slice(wrongTimes, func(i, j int) bool { return wrongTimes[i] < wrongTimes[j] })
	if missingTimes[4] > 3*wrongTimes[4] || wrongTimes[4] > 3*missingTimes[4] {
		t.Fatalf("login median timings differ: unknown = %s, wrong password = %s", missingTimes[4], wrongTimes[4])
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET email_verified_at = NULL, retired_at = statement_timestamp() WHERE subject = $1`, "login-subject"); err != nil {
		t.Fatal(err)
	}
	result, err = service.CreatePasswordSession(ctx, input)
	if !errors.Is(err, app.ErrInvalidCredentials) || result.AccessToken != "" {
		t.Fatalf("retired login returned %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_sessions WHERE account_subject = $1`, "login-subject").Scan(&count); err != nil || count != 2 {
		t.Fatalf("final session count = %d, error = %v", count, err)
	}
}

func testClaimCode(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Run("claim code requests keep public responses generic and purposes separate", func(t *testing.T) {
		queries := identitysqlc.New(pool)
		key := bytes.Repeat([]byte{9}, 32)
		protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{10}, 32), 1)
		if err != nil {
			t.Fatal(err)
		}
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
		service := app.NewClaimCodeService(identitypostgres.NewAccountRepository(pool), protector, limits.CodeRequest, key)
		handler := identityhttp.NewIdentityHandler(nil, nil, service, nil, nil, nil)
		request := func(email string) error {
			t.Helper()
			ctx := peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.111"), Port: 1234}})
			response, err := handler.RequestUnverifiedAccountClaimCode(ctx, &identityv1.RequestUnverifiedAccountClaimCodeRequest{Email: email})
			if err == nil && !response.GetAccepted() {
				t.Fatal("request was not accepted")
			}
			return err
		}
		account, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "claim-code-main", EmailLocal: "ClaimCodeMain", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, verificationVerifier, _, err := domain.NewChallenge(key, account.Subject, "ClaimCodeMain@example.com", domain.PurposeVerifyEmail, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		verification, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
			AccountSubject: account.Subject, Purpose: "verify-email", EmailLocal: "ClaimCodeMain",
			EmailDomain: "example.com", CodeVerifier: verificationVerifier[:],
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := request("ClaimCodeMain@EXAMPLE.COM"); err != nil {
			t.Fatalf("shared cooldown response = %v", err)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE account_subject = $1 AND purpose = 'claim-account'`, account.Subject).Scan(&count); err != nil || count != 0 {
			t.Fatalf("claim during shared cooldown = %d, %v", count, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at = issued_at - INTERVAL '61 seconds', expires_at = expires_at - INTERVAL '61 seconds' WHERE id = $1`, verification.ID); err != nil {
			t.Fatal(err)
		}
		if err := request("ClaimCodeMain@example.com"); err != nil {
			t.Fatalf("eligible response = %v", err)
		}
		var first pgtype.UUID
		if err := pool.QueryRow(ctx, `SELECT id FROM identity_challenges WHERE account_subject = $1 AND purpose = 'claim-account' AND replaced_at IS NULL`, account.Subject).Scan(&first); err != nil {
			t.Fatal(err)
		}
		if err := request("ClaimCodeMain@example.com"); err != nil {
			t.Fatalf("claim resend cooldown response = %v", err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE account_subject = $1 AND purpose = 'claim-account'`, account.Subject).Scan(&count); err != nil || count != 1 {
			t.Fatalf("claim resend during cooldown = %d, %v", count, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at = issued_at - INTERVAL '61 seconds', expires_at = expires_at - INTERVAL '61 seconds' WHERE id = $1`, first); err != nil {
			t.Fatal(err)
		}
		failed := app.NewClaimCodeService(identitypostgres.NewAccountRepository(pool), failingDeliveryProtector{}, limits.CodeRequest, key)
		if err := failed.RequestUnverifiedAccountClaimCode(ctx, inbound.RequestClaimCodeInput{
			Email: "ClaimCodeMain@example.com", Source: "192.0.2.111",
		}); !errors.Is(err, app.ErrClaimCodeUnavailable) {
			t.Fatalf("delivery protection failure = %v", err)
		}
		var stillCurrent pgtype.UUID
		if err := pool.QueryRow(ctx, `SELECT id FROM identity_challenges WHERE account_subject = $1 AND purpose = 'claim-account' AND replaced_at IS NULL`, account.Subject).Scan(&stillCurrent); err != nil || stillCurrent != first {
			t.Fatalf("failed replacement current challenge = %v, error = %v", stillCurrent, err)
		}
		if err := request("ClaimCodeMain@example.com"); err != nil {
			t.Fatalf("claim replacement response = %v", err)
		}
		var current pgtype.UUID
		var verificationReplaced, firstReplaced sql.NullTime
		if err := pool.QueryRow(ctx, `SELECT id FROM identity_challenges WHERE account_subject = $1 AND purpose = 'claim-account' AND replaced_at IS NULL`, account.Subject).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT replaced_at FROM identity_challenges WHERE id = $1`, verification.ID).Scan(&verificationReplaced); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT replaced_at FROM identity_challenges WHERE id = $1`, first).Scan(&firstReplaced); err != nil || current == first || verificationReplaced.Valid || !firstReplaced.Valid {
			t.Fatalf("purpose isolation = new %v, verify replaced %v, claim replaced %v, error %v", current, verificationReplaced, firstReplaced, err)
		}
		if err := identitypostgres.NewDeliveryRepository(pool).WithCurrentDelivery(ctx, uuid.UUID(first.Bytes).String(), "claim-account", func(context.Context, outbound.CurrentDelivery) error {
			t.Fatal("replaced claim code was sent")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := identitypostgres.NewDeliveryRepository(pool).WithCurrentDelivery(ctx, uuid.UUID(current.Bytes).String(), "claim-account", func(_ context.Context, delivery outbound.CurrentDelivery) error {
			email, code, err := protector.Open(uuid.UUID(current.Bytes).String(), "claim-account", account.Subject, delivery.Material)
			if err != nil || email != "ClaimCodeMain@example.com" || len(code) != 6 {
				t.Fatalf("claim delivery = %q, code length %d, error %v", email, len(code), err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		var eventCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events WHERE challenge_id = $1`, current).Scan(&eventCount); err != nil || eventCount != 1 {
			t.Fatalf("claim outbox event count = %d, error = %v", eventCount, err)
		}
		for _, name := range []string{"missing", "verified"} {
			if name == "verified" {
				if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
					Subject: "claim-code-verified", EmailLocal: "ClaimCodeVerified", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := queries.MarkEmailVerified(ctx, "claim-code-verified"); err != nil {
					t.Fatal(err)
				}
			}
			var before int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			address := "ClaimCodeMissing@example.com"
			if name == "verified" {
				address = "ClaimCodeVerified@example.com"
			}
			if err := request(address); err != nil {
				t.Fatalf("%s response = %v", name, err)
			}
			var after int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events`).Scan(&after); err != nil || after != before {
				t.Fatalf("%s outbox count %d -> %d, error %v", name, before, after, err)
			}
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at = issued_at - INTERVAL '61 seconds', expires_at = expires_at - INTERVAL '61 seconds' WHERE id = $1`, current); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if _, err := pool.Exec(ctx, `INSERT INTO identity_challenges (account_subject, purpose, email_local, email_domain, code_verifier, issued_at, expires_at, replaced_at)
				VALUES ($1, 'claim-account', 'ClaimCodeMain', 'example.com', $2, statement_timestamp() - INTERVAL '5 minutes', statement_timestamp() + INTERVAL '5 minutes', statement_timestamp())`,
				account.Subject, bytes.Repeat([]byte{6}, 32)); err != nil {
				t.Fatal(err)
			}
		}
		if err := request("ClaimCodeMain@example.com"); err != nil {
			t.Fatalf("shared account hourly limit response = %v", err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE account_subject = $1`, account.Subject).Scan(&count); err != nil || count != 5 {
			t.Fatalf("shared account hourly limit count = %d, error = %v", count, err)
		}
		for range 60 {
			if err := limits.CodeRequest(ctx, "192.0.2.112"); err != nil {
				t.Fatal(err)
			}
		}
		if err := service.RequestUnverifiedAccountClaimCode(ctx, inbound.RequestClaimCodeInput{
			Email: "ClaimCodeMain@example.com", Source: "192.0.2.112",
		}); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("shared source limit = %v", err)
		}
		durations := map[string][]time.Duration{"missing": {}, "verified": {}, "eligible": {}}
		for i := range 5 {
			for _, state := range []string{"missing", "verified", "eligible"} {
				local := "ClaimTiming" + state + strconv.Itoa(i)
				if state != "missing" {
					subject := "claim-timing-" + state + strconv.Itoa(i)
					if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
						Subject: subject, EmailLocal: local, EmailDomain: "example.com", PasswordHash: "$argon2id$test",
					}); err != nil {
						t.Fatal(err)
					}
					if state == "verified" {
						if _, err := queries.MarkEmailVerified(ctx, subject); err != nil {
							t.Fatal(err)
						}
					}
				}
				started := time.Now()
				if err := request(local + "@example.com"); err != nil {
					t.Fatalf("timing request for %s = %v", state, err)
				}
				durations[state] = append(durations[state], time.Since(started))
			}
		}
		var fastest, slowest time.Duration
		for state, samples := range durations {
			slices.Sort(samples)
			median := samples[len(samples)/2]
			if median < 100*time.Millisecond {
				t.Fatalf("%s median response %s is below the timing floor", state, median)
			}
			if fastest == 0 || median < fastest {
				fastest = median
			}
			if median > slowest {
				slowest = median
			}
		}
		if slowest-fastest > 60*time.Millisecond {
			t.Fatalf("public response medians differ by %s: %v", slowest-fastest, durations)
		}
	})
}

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
	verification := app.NewEmailVerificationService(repository, limits.WrongCode, limits.AccountWrongCode, key)

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
		login := app.NewPasswordLoginService(repository, signer, func(context.Context, string, string) error { return nil })
		recovered, err := login.CreatePasswordSession(ctx, inbound.CreatePasswordSessionInput{
			Email: old.email, Password: old.input().NewPassword, Source: "192.0.2.130",
		})
		if err != nil || recovered.Subject != result.Subject || !recovered.EmailVerified ||
			recovered.RefreshToken == result.RefreshToken || recovered.AccessToken == "" {
			t.Fatalf("login after lost claim response failed: %v", err)
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

func testClaimRetirement(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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
}

func testVerificationCodeResend(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Run("verification resend replaces only its challenge and keeps a durable delivery", func(t *testing.T) {
		const subject = "resend-subject"
		queries := identitysqlc.New(pool)
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: subject, EmailLocal: "Resend", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: bytes.Repeat([]byte{9}, 32)})
		if err != nil {
			t.Fatal(err)
		}
		old, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
			AccountSubject: subject, Purpose: "verify-email", EmailLocal: "Resend", EmailDomain: "example.com", CodeVerifier: bytes.Repeat([]byte{1}, 32),
		})
		if err != nil {
			t.Fatal(err)
		}
		claim, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
			AccountSubject: subject, Purpose: "claim-account", EmailLocal: "Resend", EmailDomain: "example.com", CodeVerifier: bytes.Repeat([]byte{2}, 32),
		})
		if err != nil {
			t.Fatal(err)
		}
		material := outbound.DeliveryMaterial{KeyVersion: 1, Nonce: bytes.Repeat([]byte{2}, 12), Ciphertext: bytes.Repeat([]byte{3}, 17)}
		if err := queries.StoreChallengeDelivery(ctx, identitysqlc.StoreChallengeDeliveryParams{
			ChallengeID: old.ID, KeyVersion: material.KeyVersion, Nonce: material.Nonce, Ciphertext: material.Ciphertext,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := queries.CreateOutboxEvent(ctx, old.ID); err != nil {
			t.Fatal(err)
		}
		protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{3}, 32), 1)
		if err != nil {
			t.Fatal(err)
		}
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
		service := app.NewEmailVerificationCodeService(identitypostgres.NewAccountRepository(pool), protector, limits.CodeRequest, bytes.Repeat([]byte{4}, 32))
		input := inbound.RequestEmailVerificationCodeInput{Subject: subject, SessionID: uuid.UUID(session.ID.Bytes).String(), Source: "192.0.2.91"}
		if err := service.RequestEmailVerificationCode(ctx, input); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("immediate resend = %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at = issued_at - INTERVAL '61 seconds', expires_at = expires_at - INTERVAL '61 seconds' WHERE id IN ($1, $2)`, old.ID, claim.ID); err != nil {
			t.Fatal(err)
		}
		if err := service.RequestEmailVerificationCode(ctx, input); err != nil {
			t.Fatalf("resend = %v", err)
		}
		var current pgtype.UUID
		var oldReplaced, claimReplaced sql.NullTime
		var outboxCount int
		if err := pool.QueryRow(ctx, `SELECT id FROM identity_challenges WHERE account_subject = $1 AND purpose = 'verify-email' AND replaced_at IS NULL`, subject).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT replaced_at FROM identity_challenges WHERE id = $1`, old.ID).Scan(&oldReplaced); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT replaced_at FROM identity_challenges WHERE id = $1`, claim.ID).Scan(&claimReplaced); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events WHERE challenge_id IN ($1, $2)`, old.ID, current).Scan(&outboxCount); err != nil || outboxCount != 2 || current == old.ID || !oldReplaced.Valid || claimReplaced.Valid {
			t.Fatalf("replacement state = current %v, old %v, claim %v, outbox %d, error %v", current, oldReplaced, claimReplaced, outboxCount, err)
		}
		delivery := identitypostgres.NewDeliveryRepository(pool)
		if err := delivery.WithCurrentDelivery(ctx, uuid.UUID(old.ID.Bytes).String(), "verify-email", func(context.Context, outbound.CurrentDelivery) error {
			t.Fatal("stale code was sent")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := delivery.WithCurrentDelivery(ctx, uuid.UUID(current.Bytes).String(), "verify-email", func(_ context.Context, currentDelivery outbound.CurrentDelivery) error {
			email, code, err := protector.Open(uuid.UUID(current.Bytes).String(), "verify-email", subject, currentDelivery.Material)
			if err != nil || email != "Resend@example.com" || len(code) != 6 {
				t.Fatalf("current delivery is invalid: %v", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at = issued_at - INTERVAL '61 seconds', expires_at = expires_at - INTERVAL '61 seconds' WHERE id = $1`, current); err != nil {
			t.Fatal(err)
		}
		failed := app.NewEmailVerificationCodeService(identitypostgres.NewAccountRepository(pool), failingDeliveryProtector{}, limits.CodeRequest, bytes.Repeat([]byte{4}, 32))
		if err := failed.RequestEmailVerificationCode(ctx, input); !errors.Is(err, app.ErrVerificationCodeUnavailable) {
			t.Fatalf("failed delivery protection = %v", err)
		}
		var stillCurrent pgtype.UUID
		if err := pool.QueryRow(ctx, `SELECT id FROM identity_challenges WHERE account_subject = $1 AND purpose = 'verify-email' AND replaced_at IS NULL`, subject).Scan(&stillCurrent); err != nil || stillCurrent != current {
			t.Fatalf("rollback current challenge = %v, %v", stillCurrent, err)
		}
		for range 2 {
			if _, err := pool.Exec(ctx, `INSERT INTO identity_challenges (account_subject, purpose, email_local, email_domain, code_verifier, issued_at, expires_at, replaced_at)
				VALUES ($1, 'claim-account', 'Resend', 'example.com', $2, statement_timestamp() - INTERVAL '5 minutes', statement_timestamp() + INTERVAL '5 minutes', statement_timestamp())`, subject, bytes.Repeat([]byte{5}, 32)); err != nil {
				t.Fatal(err)
			}
		}
		if err := service.RequestEmailVerificationCode(ctx, input); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("sixth code in hour = %v", err)
		}
		if _, err := queries.MarkEmailVerified(ctx, subject); err != nil {
			t.Fatal(err)
		}
		if err := service.RequestEmailVerificationCode(ctx, input); !errors.Is(err, app.ErrEmailAlreadyVerified) {
			t.Fatalf("verified account = %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET revoked_at = statement_timestamp() WHERE id = $1`, session.ID); err != nil {
			t.Fatal(err)
		}
		if err := service.RequestEmailVerificationCode(ctx, input); !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatalf("revoked session = %v", err)
		}
	})
}

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

func testRequestPasswordResetCode(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Run("recovery request commits one current challenge and outbox event", func(t *testing.T) {
		queries := identitysqlc.New(pool)
		account, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "recovery-request", EmailLocal: "Recovery", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := queries.MarkEmailVerified(ctx, account.Subject); err != nil {
			t.Fatal(err)
		}
		for _, purpose := range []string{"verify-email", "claim-account"} {
			if _, err := pool.Exec(ctx, `INSERT INTO identity_challenges (account_subject,purpose,email_local,email_domain,code_verifier,issued_at,expires_at)
				VALUES ($1,$2,'Recovery','example.com',$3,statement_timestamp()-INTERVAL '61 seconds',statement_timestamp()+INTERVAL '539 seconds')`,
				account.Subject, purpose, bytes.Repeat([]byte{1}, 32)); err != nil {
				t.Fatal(err)
			}
		}
		protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{3}, 32), 1)
		if err != nil {
			t.Fatal(err)
		}
		key := bytes.Repeat([]byte{4}, 32)
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
		service := app.NewPasswordResetCodeService(identitypostgres.NewAccountRepository(pool), protector, limits.CodeRequest,
			func(ctx context.Context, email string) error { return limits.PasswordRecoveryEmail(ctx, email, key) }, key)
		input := inbound.RequestPasswordResetCodeInput{Email: "Recovery@EXAMPLE.COM", Source: "192.0.2.199"}
		request := func() {
			t.Helper()
			if err := service.RequestPasswordResetCode(ctx, input); err != nil {
				t.Fatal(err)
			}
		}
		request()
		var verifier, nonce, ciphertext []byte
		var challengeID string
		if err := pool.QueryRow(ctx, `SELECT c.id::text,c.code_verifier,d.nonce,d.ciphertext FROM identity_challenges c
			JOIN identity_challenge_deliveries d ON d.challenge_id=c.id WHERE c.account_subject=$1 AND c.purpose='password-reset' AND c.replaced_at IS NULL`, account.Subject).
			Scan(&challengeID, &verifier, &nonce, &ciphertext); err != nil {
			t.Fatal(err)
		}
		if len(verifier) != 32 || len(nonce) != 12 || len(ciphertext) <= 16 {
			t.Fatal("challenge material is incomplete")
		}
		if err := identitypostgres.NewDeliveryRepository(pool).PurgeTerminal(ctx); err != nil {
			t.Fatal(err)
		}
		var queuedMaterial int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenge_deliveries WHERE challenge_id=$1::uuid`, challengeID).Scan(&queuedMaterial); err != nil || queuedMaterial != 1 {
			t.Fatalf("cleanup removed current recovery material: count=%d error=%v", queuedMaterial, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE identity_outbox_events AS event SET next_attempt_at=statement_timestamp()+INTERVAL '1 hour'
			FROM identity_challenges AS challenge WHERE challenge.id=event.challenge_id AND challenge.purpose<>'password-reset'`); err != nil {
			t.Fatal(err)
		}
		claimed, err := identitysqlc.New(tx).ClaimOutboxEvent(ctx, pgtype.Text{String: "recovery-test", Valid: true})
		if err != nil || uuid.UUID(claimed.ChallengeID.Bytes).String() != challengeID || claimed.Purpose != string(domain.PurposePasswordReset) {
			t.Fatalf("relay did not claim recovery event: challenge=%v purpose=%q error=%v", claimed.ChallengeID, claimed.Purpose, err)
		}
		if _, err := identitysqlc.New(tx).ClaimOutboxEvent(ctx, pgtype.Text{String: "recovery-test", Valid: true}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("relay claimed recovery event: %v", err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		email, code, err := protector.Open(challengeID, string(domain.PurposePasswordReset), account.Subject,
			outbound.DeliveryMaterial{KeyVersion: 1, Nonce: nonce, Ciphertext: ciphertext})
		if err != nil || email != "Recovery@example.com" || len(code) != 6 {
			t.Fatalf("delivery=%q code length=%d error=%v", email, len(code), err)
		}
		var stored [32]byte
		copy(stored[:], verifier)
		if !domain.VerifyChallenge(key, account.Subject, email, domain.PurposePasswordReset, code, stored, time.Now().Add(time.Minute), time.Now()) ||
			domain.VerifyChallenge(key, account.Subject, email, domain.PurposeVerifyEmail, code, stored, time.Now().Add(time.Minute), time.Now()) {
			t.Fatal("purpose binding failed")
		}
		request()
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE account_subject=$1 AND purpose='password-reset'`, account.Subject).Scan(&count); err != nil || count != 1 {
			t.Fatalf("cooldown count=%d error=%v", count, err)
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at=issued_at-INTERVAL '61 seconds',expires_at=expires_at-INTERVAL '61 seconds' WHERE id=$1::uuid`, challengeID); err != nil {
			t.Fatal(err)
		}
		failed := app.NewPasswordResetCodeService(identitypostgres.NewAccountRepository(pool), failingDeliveryProtector{}, limits.CodeRequest,
			func(ctx context.Context, email string) error { return limits.PasswordRecoveryEmail(ctx, email, key) }, key)
		if err := failed.RequestPasswordResetCode(ctx, input); !errors.Is(err, app.ErrPasswordResetCodeUnavailable) {
			t.Fatalf("failed transaction=%v", err)
		}
		var afterFailure string
		if err := pool.QueryRow(ctx, `SELECT id::text FROM identity_challenges WHERE account_subject=$1 AND purpose='password-reset' AND replaced_at IS NULL`, account.Subject).Scan(&afterFailure); err != nil || afterFailure != challengeID {
			t.Fatalf("rollback current=%q error=%v", afterFailure, err)
		}
		request()
		var currentID string
		if err := pool.QueryRow(ctx, `SELECT id::text FROM identity_challenges WHERE account_subject=$1 AND purpose='password-reset' AND replaced_at IS NULL`, account.Subject).Scan(&currentID); err != nil || currentID == challengeID {
			t.Fatalf("current=%q error=%v", currentID, err)
		}
		var outboxCount, oldDelivery, otherCurrent int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_outbox_events WHERE challenge_id IN ($1::uuid,$2::uuid)`, challengeID, currentID).Scan(&outboxCount); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenge_deliveries WHERE challenge_id=$1::uuid`, challengeID).Scan(&oldDelivery); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE account_subject=$1 AND purpose IN ('verify-email','claim-account') AND replaced_at IS NULL`, account.Subject).Scan(&otherCurrent); err != nil {
			t.Fatal(err)
		}
		if outboxCount != 2 || oldDelivery != 0 || otherCurrent != 2 {
			t.Fatalf("outbox=%d old delivery=%d other current=%d", outboxCount, oldDelivery, otherCurrent)
		}
	})

	t.Run("missing and unverified accounts consume shared limits without mail", func(t *testing.T) {
		queries := identitysqlc.New(pool)
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: "recovery-unverified", EmailLocal: "RecoveryUnverified", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{5}, 32), 1)
		if err != nil {
			t.Fatal(err)
		}
		key := bytes.Repeat([]byte{6}, 32)
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
		newService := func() *app.PasswordResetCodeService {
			return app.NewPasswordResetCodeService(identitypostgres.NewAccountRepository(pool), protector, limits.CodeRequest,
				func(ctx context.Context, email string) error { return limits.PasswordRecoveryEmail(ctx, email, key) }, key)
		}
		for _, email := range []string{"Missing@example.com", "RecoveryUnverified@example.com"} {
			if err := newService().RequestPasswordResetCode(ctx, inbound.RequestPasswordResetCodeInput{Email: email, Source: "192.0.2.200"}); err != nil {
				t.Fatal(err)
			}
		}
		var challenges, emailCounters int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE account_subject='recovery-unverified'`).Scan(&challenges); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_limit_counters WHERE scope='email' AND action='code-request' AND counter_key NOT LIKE '%@%'`).Scan(&emailCounters); err != nil {
			t.Fatal(err)
		}
		if challenges != 0 || emailCounters < 2 {
			t.Fatalf("challenges=%d keyed counters=%d", challenges, emailCounters)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO identity_limit_counters(scope,counter_key,action,window_start,count)
			VALUES('source','192.0.2.201','code-request',statement_timestamp(),59)`); err != nil {
			t.Fatal(err)
		}
		input := inbound.RequestPasswordResetCodeInput{Email: "Missing@example.com", Source: "192.0.2.201"}
		if err := newService().RequestPasswordResetCode(ctx, input); err != nil {
			t.Fatalf("60th request=%v", err)
		}
		if err := newService().RequestPasswordResetCode(ctx, input); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("61st request=%v", err)
		}
	})

	t.Run("recovery requests keep practical timing and leave credentials unchanged", func(t *testing.T) {
		queries := identitysqlc.New(pool)
		protector, err := deliverycrypto.NewDeliveryProtector(bytes.Repeat([]byte{7}, 32), 1)
		if err != nil {
			t.Fatal(err)
		}
		key := bytes.Repeat([]byte{8}, 32)
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
		service := app.NewPasswordResetCodeService(identitypostgres.NewAccountRepository(pool), protector, limits.CodeRequest,
			func(ctx context.Context, email string) error { return limits.PasswordRecoveryEmail(ctx, email, key) }, key)
		durations := map[string][]time.Duration{"missing": {}, "unverified": {}, "eligible": {}}
		for i := range 5 {
			for _, state := range []string{"missing", "unverified", "eligible"} {
				local := "RecoveryTiming" + state + strconv.Itoa(i)
				subject := "recovery-timing-" + state + strconv.Itoa(i)
				if state != "missing" {
					if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
						Subject: subject, EmailLocal: local, EmailDomain: "example.com", PasswordHash: "$argon2id$timing",
					}); err != nil {
						t.Fatal(err)
					}
					refreshHash := sha256.Sum256([]byte(subject))
					if _, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{
						AccountSubject: subject, RefreshTokenHash: refreshHash[:],
					}); err != nil {
						t.Fatal(err)
					}
				}
				if state == "eligible" {
					if _, err := queries.MarkEmailVerified(ctx, subject); err != nil {
						t.Fatal(err)
					}
				}
				started := time.Now()
				if err := service.RequestPasswordResetCode(ctx, inbound.RequestPasswordResetCodeInput{
					Email: local + "@example.com", Source: "192.0.2.210",
				}); err != nil {
					t.Fatalf("timing request for %s = %v", state, err)
				}
				durations[state] = append(durations[state], time.Since(started))
				if state == "missing" {
					continue
				}
				var hash string
				var verified bool
				var activeSessions, challenges int
				if err := pool.QueryRow(ctx, `SELECT password_hash, email_verified_at IS NOT NULL,
					(SELECT count(*) FROM identity_sessions WHERE account_subject=$1 AND revoked_at IS NULL),
					(SELECT count(*) FROM identity_challenges WHERE account_subject=$1 AND purpose='password-reset')
					FROM identity_accounts WHERE subject=$1`, subject).Scan(&hash, &verified, &activeSessions, &challenges); err != nil {
					t.Fatal(err)
				}
				wantChallenges := map[string]int{"unverified": 0, "eligible": 1}[state]
				if hash != "$argon2id$timing" || verified != (state == "eligible") || activeSessions != 1 || challenges != wantChallenges {
					t.Fatalf("%s request changed account state: verified=%v sessions=%d challenges=%d", state, verified, activeSessions, challenges)
				}
			}
		}
		var fastest, slowest time.Duration
		for state, samples := range durations {
			slices.Sort(samples)
			median := samples[len(samples)/2]
			if median < 100*time.Millisecond {
				t.Fatalf("%s median response %s is below the timing floor", state, median)
			}
			if fastest == 0 || median < fastest {
				fastest = median
			}
			if median > slowest {
				slowest = median
			}
		}
		if slowest-fastest > 60*time.Millisecond {
			t.Fatalf("recovery response medians differ by %s: %v", slowest-fastest, durations)
		}
	})
}

func testAllSessionLogout(t *testing.T, pool *pgxpool.Pool) {
	ctx := t.Context()
	queries := identitysqlc.New(pool)
	createAccount := func(subject string) {
		t.Helper()
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
	}
	createSession := func(subject string, seed byte) (string, string) {
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
	createAccount("all-logout-subject")
	createAccount("all-logout-other")
	currentID, currentRefresh := createSession("all-logout-subject", 21)
	otherID, otherRefresh := createSession("all-logout-subject", 22)
	unrelatedID, unrelatedRefresh := createSession("all-logout-other", 23)
	logout := app.NewAllSessionLogoutService(identitypostgres.NewAccountRepository(pool))
	check := app.NewSessionCheckService(identitypostgres.NewSessionRepository(pool))
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewSigner(privateKey, "all-logout-key", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	refresh := app.NewSessionRefreshService(identitypostgres.NewSessionRefreshRepository(pool), signer)
	input := inbound.LogoutAllSessionsInput{Subject: "all-logout-subject", SessionID: currentID}
	if err := logout.LogoutAllSessions(ctx, inbound.LogoutAllSessionsInput{Subject: "all-logout-other", SessionID: currentID}); !errors.Is(err, outbound.ErrUnauthenticated) {
		t.Fatalf("wrong subject = %v", err)
	}
	if err := logout.LogoutAllSessions(ctx, input); err != nil {
		t.Fatalf("logout = %v", err)
	}
	for _, id := range []string{currentID, otherID} {
		if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: input.Subject, SessionID: id}); !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatalf("revoked session %s = %v", id, err)
		}
	}
	for _, raw := range []string{currentRefresh, otherRefresh} {
		if _, err := refresh.RefreshSession(ctx, raw); !errors.Is(err, app.ErrUnauthenticatedRefresh) {
			t.Fatalf("revoked refresh = %v", err)
		}
	}
	if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: "all-logout-other", SessionID: unrelatedID}); err != nil {
		t.Fatalf("other account session = %v", err)
	}
	if _, err := refresh.RefreshSession(ctx, unrelatedRefresh); err != nil {
		t.Fatalf("other account refresh = %v", err)
	}
	if err := logout.LogoutAllSessions(ctx, input); !errors.Is(err, outbound.ErrUnauthenticated) {
		t.Fatalf("repeated logout = %v", err)
	}
	var laterID string
	if err := identitypostgres.NewAccountRepository(pool).WithinPasswordSessionTransaction(ctx, func(tx outbound.PasswordSessionTransaction) error {
		if _, err := tx.LockPasswordAccount(ctx, input.Subject); err != nil {
			return err
		}
		hash := sha256.Sum256([]byte("login-after-logout"))
		session, err := tx.Create(ctx, input.Subject, hash[:])
		laterID = session.ID
		return err
	}); err != nil {
		t.Fatalf("later login = %v", err)
	}
	if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: input.Subject, SessionID: laterID}); err != nil {
		t.Fatalf("later login remained revoked: %v", err)
	}
	t.Run("failed revocation rolls back", func(t *testing.T) {
		const subject = "all-logout-rollback"
		createAccount(subject)
		currentID, _ := createSession(subject, 28)
		otherID, _ := createSession(subject, 29)
		lock, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = lock.Rollback(ctx) }()
		if _, err := lock.Exec(ctx, `SELECT id FROM identity_sessions WHERE id = $1 FOR UPDATE`, otherID); err != nil {
			t.Fatal(err)
		}
		deadline, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		err = logout.LogoutAllSessions(deadline, inbound.LogoutAllSessionsInput{Subject: subject, SessionID: currentID})
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Fatalf("failed transaction = %v", err)
		}
		if err := lock.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{currentID, otherID} {
			if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: subject, SessionID: id}); err != nil {
				t.Fatalf("transaction failure revoked session %s: %v", id, err)
			}
		}
	})
	closedPool, err := pgxpool.New(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	closedPool.Close()
	if err := app.NewAllSessionLogoutService(identitypostgres.NewAccountRepository(closedPool)).LogoutAllSessions(ctx,
		inbound.LogoutAllSessionsInput{Subject: "all-logout-other", SessionID: unrelatedID}); !errors.Is(err, app.ErrAllLogoutUnavailable) {
		t.Fatalf("database failure = %v", err)
	}

	t.Run("login commits before logout", func(t *testing.T) {
		const subject = "all-logout-login-first"
		createAccount(subject)
		currentID, _ := createSession(subject, 24)
		releaseLogin := make(chan struct{})
		created := make(chan string, 1)
		loginDone := make(chan error, 1)
		go func() {
			loginDone <- identitypostgres.NewAccountRepository(pool).WithinPasswordSessionTransaction(ctx, func(tx outbound.PasswordSessionTransaction) error {
				if _, err := tx.LockPasswordAccount(ctx, subject); err != nil {
					return err
				}
				hash := sha256.Sum256([]byte("login-before-logout"))
				session, err := tx.Create(ctx, subject, hash[:])
				if err != nil {
					return err
				}
				created <- session.ID
				<-releaseLogin
				return nil
			})
		}()
		var newID string
		select {
		case newID = <-created:
		case err := <-loginDone:
			t.Fatalf("login failed before session creation: %v", err)
		}
		logoutDone := make(chan error, 1)
		go func() {
			logoutDone <- logout.LogoutAllSessions(ctx, inbound.LogoutAllSessionsInput{Subject: subject, SessionID: currentID})
		}()
		select {
		case err := <-logoutDone:
			close(releaseLogin)
			t.Fatalf("logout committed before the pending login: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		close(releaseLogin)
		if err := <-loginDone; err != nil {
			t.Fatal(err)
		}
		if err := <-logoutDone; err != nil {
			t.Fatal(err)
		}
		if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: subject, SessionID: newID}); !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatalf("earlier login survived logout: %v", err)
		}
	})

	t.Run("logout commits before login", func(t *testing.T) {
		const subject = "all-logout-logout-first"
		createAccount(subject)
		currentID, _ := createSession(subject, 25)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		locked := identitysqlc.New(tx)
		id, err := uuid.Parse(currentID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := locked.GetActiveAccountForSessionForUpdate(ctx, identitysqlc.GetActiveAccountForSessionForUpdateParams{
			Subject: subject, SessionID: pgtype.UUID{Bytes: id, Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := locked.RevokeAccountSessions(ctx, subject); err != nil {
			t.Fatal(err)
		}
		loginStarted := make(chan struct{})
		type loginResult struct {
			id  string
			err error
		}
		loginDone := make(chan loginResult, 1)
		go func() {
			var newID string
			err := identitypostgres.NewAccountRepository(pool).WithinPasswordSessionTransaction(ctx, func(loginTx outbound.PasswordSessionTransaction) error {
				close(loginStarted)
				if _, err := loginTx.LockPasswordAccount(ctx, subject); err != nil {
					return err
				}
				hash := sha256.Sum256([]byte("logout-before-login"))
				session, err := loginTx.Create(ctx, subject, hash[:])
				newID = session.ID
				return err
			})
			loginDone <- loginResult{id: newID, err: err}
		}()
		<-loginStarted
		select {
		case result := <-loginDone:
			t.Fatalf("login committed before logout: %+v", result)
		case <-time.After(100 * time.Millisecond):
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		result := <-loginDone
		if result.err != nil {
			t.Fatal(result.err)
		}
		if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: subject, SessionID: result.id}); err != nil {
			t.Fatalf("later login did not survive: %v", err)
		}
		if _, err := check.CheckSession(ctx, inbound.CheckSessionInput{Subject: subject, SessionID: currentID}); !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatalf("old session survived logout: %v", err)
		}
	})
}
