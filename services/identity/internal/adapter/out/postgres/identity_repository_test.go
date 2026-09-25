//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
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

	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
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

func TestDeliveryRepository(t *testing.T) {
	ctx := context.Background()
	container, err := postgrescontainer.Run(ctx, "postgres:18-alpine",
		postgrescontainer.WithDatabase("identity"), postgrescontainer.WithUsername("identity"),
		postgrescontainer.WithPassword("identity"), postgrescontainer.BasicWaitStrategies())
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
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	queries := identitysqlc.New(pool)
	if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
		Subject: "delivery-subject", EmailLocal: "Recipient", EmailDomain: "example.com", PasswordHash: "$argon2id$test",
	}); err != nil {
		t.Fatal(err)
	}
	challenge, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
		AccountSubject: "delivery-subject", Purpose: "verify-email", EmailLocal: "Recipient",
		EmailDomain: "example.com", CodeVerifier: bytes.Repeat([]byte{1}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	material := outbound.DeliveryMaterial{KeyVersion: 1, Nonce: bytes.Repeat([]byte{2}, 12), Ciphertext: bytes.Repeat([]byte{3}, 17)}
	if err := queries.StoreChallengeDelivery(ctx, identitysqlc.StoreChallengeDeliveryParams{
		ChallengeID: challenge.ID, KeyVersion: material.KeyVersion, Nonce: material.Nonce, Ciphertext: material.Ciphertext,
	}); err != nil {
		t.Fatal(err)
	}
	challengeID := uuid.UUID(challenge.ID.Bytes).String()
	repository := identitypostgres.NewDeliveryRepository(pool)
	if err := repository.WithCurrentDelivery(ctx, "bad-id", "verify-email", nil); err == nil {
		t.Fatal("invalid delivery challenge ID accepted")
	}
	if err := repository.WithCurrentDelivery(ctx, uuid.NewString(), "verify-email", nil); err != nil {
		t.Fatalf("missing delivery challenge = %v", err)
	}
	mailFailure := errors.New("mail server unavailable")
	sends := 0
	send := func(_ context.Context, delivery outbound.CurrentDelivery) error {
		if delivery.Subject != "delivery-subject" || delivery.Email != "Recipient@example.com" || !bytes.Equal(delivery.Material.Ciphertext, material.Ciphertext) {
			t.Fatal("unexpected delivery data")
		}
		sends++
		if sends == 1 {
			return mailFailure
		}
		return nil
	}
	if err := repository.WithCurrentDelivery(ctx, challengeID, "verify-email", send); !errors.Is(err, mailFailure) {
		t.Fatalf("mail failure = %v", err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenge_deliveries WHERE challenge_id = $1`, challenge.ID).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("retry material after mail failure = %d, %v", remaining, err)
	}
	if err := repository.WithCurrentDelivery(ctx, challengeID, "verify-email", send); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenge_deliveries WHERE challenge_id = $1`, challenge.ID).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("retry material after success = %d, %v", remaining, err)
	}
	if err := repository.WithCurrentDelivery(ctx, challengeID, "verify-email", send); err != nil || sends != 2 {
		t.Fatalf("replay sent %d messages, %v", sends, err)
	}
	seed := func(subject string) pgtype.UUID {
		t.Helper()
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		created, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
			AccountSubject: subject, Purpose: "verify-email", EmailLocal: subject,
			EmailDomain: "example.com", CodeVerifier: bytes.Repeat([]byte{1}, 32),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := queries.StoreChallengeDelivery(ctx, identitysqlc.StoreChallengeDeliveryParams{
			ChallengeID: created.ID, KeyVersion: material.KeyVersion, Nonce: material.Nonce, Ciphertext: material.Ciphertext,
		}); err != nil {
			t.Fatal(err)
		}
		return created.ID
	}
	countMaterial := func(id pgtype.UUID, want int) {
		t.Helper()
		var got int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenge_deliveries WHERE challenge_id = $1`, id).Scan(&got); err != nil || got != want {
			t.Fatalf("delivery material count = %d, %v; want %d", got, err, want)
		}
	}
	neverSend := func(context.Context, outbound.CurrentDelivery) error {
		t.Fatal("stale delivery sent mail")
		return nil
	}
	wrongPurpose := seed("wrong-purpose")
	if err := repository.WithCurrentDelivery(ctx, uuid.UUID(wrongPurpose.Bytes).String(), "claim-account", neverSend); err != nil {
		t.Fatal(err)
	}
	countMaterial(wrongPurpose, 1)
	replaced := seed("replaced")
	if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET replaced_at = statement_timestamp() WHERE id = $1`, replaced); err != nil {
		t.Fatal(err)
	}
	if err := repository.WithCurrentDelivery(ctx, uuid.UUID(replaced.Bytes).String(), "verify-email", neverSend); err != nil {
		t.Fatal(err)
	}
	countMaterial(replaced, 0)
	verified := seed("verified")
	if _, err := queries.MarkEmailVerified(ctx, "verified"); err != nil {
		t.Fatal(err)
	}
	if err := repository.WithCurrentDelivery(ctx, uuid.UUID(verified.Bytes).String(), "verify-email", neverSend); err != nil {
		t.Fatal(err)
	}
	countMaterial(verified, 0)
	expired := seed("expired")
	if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at = statement_timestamp() - INTERVAL '11 minutes', expires_at = statement_timestamp() - INTERVAL '1 minute' WHERE id = $1`, expired); err != nil {
		t.Fatal(err)
	}
	blocked := seed("too-many-guesses")
	if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET wrong_guesses = 5 WHERE id = $1`, blocked); err != nil {
		t.Fatal(err)
	}
	if err := repository.PurgeTerminal(ctx); err != nil {
		t.Fatal(err)
	}
	countMaterial(expired, 0)
	countMaterial(blocked, 0)
	countMaterial(wrongPurpose, 1)
	locked := seed("locked")
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- repository.WithCurrentDelivery(ctx, uuid.UUID(locked.Bytes).String(), "verify-email", func(context.Context, outbound.CurrentDelivery) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	lockCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, lockErr := queries.GetActiveAccountForUpdate(lockCtx, "locked")
	cancel()
	close(release)
	if !errors.Is(lockErr, context.DeadlineExceeded) {
		t.Fatalf("account lock during mail call = %v", lockErr)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	countMaterial(locked, 0)
}
