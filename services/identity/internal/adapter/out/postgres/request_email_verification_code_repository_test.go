//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

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
		service := app.NewRequestEmailVerificationCodeService(identitypostgres.NewAccountRepository(pool), protector, limits.CodeRequest, bytes.Repeat([]byte{4}, 32))
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
		failed := app.NewRequestEmailVerificationCodeService(identitypostgres.NewAccountRepository(pool), failingDeliveryProtector{}, limits.CodeRequest, bytes.Repeat([]byte{4}, 32))
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
