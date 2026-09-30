//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

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
	seed := func(subject, purpose string) pgtype.UUID {
		t.Helper()
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		created, err := queries.CreateChallenge(ctx, identitysqlc.CreateChallengeParams{
			AccountSubject: subject, Purpose: purpose, EmailLocal: subject,
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
	wrongPurpose := seed("wrong-purpose", "verify-email")
	if err := repository.WithCurrentDelivery(ctx, uuid.UUID(wrongPurpose.Bytes).String(), "claim-account", neverSend); err != nil {
		t.Fatal(err)
	}
	countMaterial(wrongPurpose, 1)
	replaced := seed("replaced", "verify-email")
	if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET replaced_at = statement_timestamp() WHERE id = $1`, replaced); err != nil {
		t.Fatal(err)
	}
	if err := repository.WithCurrentDelivery(ctx, uuid.UUID(replaced.Bytes).String(), "verify-email", neverSend); err != nil {
		t.Fatal(err)
	}
	countMaterial(replaced, 0)
	verified := seed("verified", "verify-email")
	if _, err := queries.MarkEmailVerified(ctx, "verified"); err != nil {
		t.Fatal(err)
	}
	if err := repository.WithCurrentDelivery(ctx, uuid.UUID(verified.Bytes).String(), "verify-email", neverSend); err != nil {
		t.Fatal(err)
	}
	countMaterial(verified, 0)
	expired := seed("expired", "verify-email")
	if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at = statement_timestamp() - INTERVAL '11 minutes', expires_at = statement_timestamp() - INTERVAL '1 minute' WHERE id = $1`, expired); err != nil {
		t.Fatal(err)
	}
	blocked := seed("too-many-guesses", "verify-email")
	if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET wrong_guesses = 5 WHERE id = $1`, blocked); err != nil {
		t.Fatal(err)
	}
	if err := repository.PurgeTerminal(ctx); err != nil {
		t.Fatal(err)
	}
	countMaterial(expired, 0)
	countMaterial(blocked, 0)
	countMaterial(wrongPurpose, 1)
	recovery := seed("recovery-current", "password-reset")
	if _, err := queries.MarkEmailVerified(ctx, "recovery-current"); err != nil {
		t.Fatal(err)
	}
	recoverySends := 0
	if err := repository.WithCurrentDelivery(ctx, uuid.UUID(recovery.Bytes).String(), "password-reset", func(_ context.Context, delivery outbound.CurrentDelivery) error {
		if delivery.Email != "recovery-current@example.com" {
			t.Fatalf("recovery recipient = %q", delivery.Email)
		}
		recoverySends++
		return nil
	}); err != nil || recoverySends != 1 {
		t.Fatalf("current recovery sends=%d error=%v", recoverySends, err)
	}
	countMaterial(recovery, 0)
	for _, test := range []struct{ name, update string }{
		{"replaced", `UPDATE identity_challenges SET replaced_at=statement_timestamp() WHERE account_subject=$1`},
		{"consumed", `UPDATE identity_challenges SET consumed_at=statement_timestamp() WHERE account_subject=$1`},
		{"expired", `UPDATE identity_challenges SET issued_at=statement_timestamp()-INTERVAL '11 minutes', expires_at=statement_timestamp()-INTERVAL '1 minute' WHERE account_subject=$1`},
		{"retired", `UPDATE identity_accounts SET email_verified_at=NULL, retired_at=statement_timestamp() WHERE subject=$1`},
	} {
		subject := "recovery-" + test.name
		id := seed(subject, "password-reset")
		if _, err := queries.MarkEmailVerified(ctx, subject); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, test.update, subject); err != nil {
			t.Fatal(err)
		}
		if err := repository.WithCurrentDelivery(ctx, uuid.UUID(id.Bytes).String(), "password-reset", neverSend); err != nil {
			t.Fatalf("%s recovery delivery: %v", test.name, err)
		}
		countMaterial(id, 0)
	}
	expiredRecovery := seed("recovery-expiry-purge", "password-reset")
	if _, err := queries.MarkEmailVerified(ctx, "recovery-expiry-purge"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at=statement_timestamp()-INTERVAL '11 minutes', expires_at=statement_timestamp()-INTERVAL '1 minute' WHERE id=$1`, expiredRecovery); err != nil {
		t.Fatal(err)
	}
	if err := repository.PurgeTerminal(ctx); err != nil {
		t.Fatal(err)
	}
	countMaterial(expiredRecovery, 0)
	locked := seed("locked", "verify-email")
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
	testPasswordChangeNoticeCleanupFailure(ctx, t, pool)
}

// testPasswordChangeNoticeCleanupFailure proves that a failed cleanup after sending keeps the notice for another attempt.
func testPasswordChangeNoticeCleanupFailure(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash, email_verified_at)
		VALUES ('notice-subject', 'Notice', 'example.com', '$argon2id$test', statement_timestamp())`); err != nil {
		t.Fatal(err)
	}
	if _, err := identitysqlc.New(pool).QueuePasswordChangeNotice(ctx, "notice-subject"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_notice_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'notice cleanup unavailable'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER reject_notice_cleanup BEFORE DELETE ON identity_password_change_notices
		FOR EACH ROW EXECUTE FUNCTION reject_notice_cleanup()`); err != nil {
		t.Fatal(err)
	}
	repository := identitypostgres.NewDeliveryRepository(pool)
	var sent []string
	send := func(_ context.Context, email string) error { sent = append(sent, email); return nil }
	if found, err := repository.WithNextPasswordChangeNotice(ctx, send); !found || err == nil {
		t.Fatalf("failed cleanup: found=%v error=%v", found, err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_notice_cleanup ON identity_password_change_notices; DROP FUNCTION reject_notice_cleanup()`); err != nil {
		t.Fatal(err)
	}
	if found, err := repository.WithNextPasswordChangeNotice(ctx, send); !found || err != nil {
		t.Fatalf("notice retry after cleanup failure: found=%v error=%v", found, err)
	}
	var retained int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_password_change_notices`).Scan(&retained); err != nil ||
		retained != 0 || len(sent) != 2 || sent[0] != "Notice@example.com" || sent[1] != sent[0] {
		t.Fatalf("notice retry: retained=%d sent=%v error=%v", retained, sent, err)
	}
}
