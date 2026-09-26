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
	"net"
	"slices"
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
		service := app.NewVerificationCodeService(identitypostgres.NewAccountRepository(pool), protector, limits.CodeRequest,
			limits.WrongCode, limits.AccountWrongCode, bytes.Repeat([]byte{4}, 32))
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
		failed := app.NewVerificationCodeService(identitypostgres.NewAccountRepository(pool), failingDeliveryProtector{}, limits.CodeRequest,
			limits.WrongCode, limits.AccountWrongCode, bytes.Repeat([]byte{4}, 32))
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

	t.Run("email verification consumes only the current code", func(t *testing.T) {
		key := bytes.Repeat([]byte{7}, 32)
		repository := identitypostgres.NewAccountRepository(pool)
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
		service := app.NewVerificationCodeService(repository, nil, nil, limits.WrongCode, limits.AccountWrongCode, key)
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
			if err := repository.WithinTransaction(ctx, func(tx outbound.AccountTransaction) error {
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

	t.Run("account claims replace one unverified identity", func(t *testing.T) {
		testAccountClaimRepository(t, pool)
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
