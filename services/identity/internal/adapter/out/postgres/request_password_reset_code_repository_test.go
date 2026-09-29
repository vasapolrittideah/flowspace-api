//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

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
}
