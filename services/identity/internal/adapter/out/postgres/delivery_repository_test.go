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
