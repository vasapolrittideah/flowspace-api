//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityevent "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/event"
	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	identityemail "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/email"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func testCodeReauthentication(t *testing.T, pool *pgxpool.Pool, db *sql.DB) {
	ctx := t.Context()
	key := bytes.Repeat([]byte{7}, 32)
	protector, err := deliverycrypto.NewDeliveryProtector(key, 1)
	if err != nil {
		t.Fatal(err)
	}
	service := func(p *pgxpool.Pool) *app.CodeReauthenticationService {
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(p))
		return app.NewCodeReauthenticationService(identitypostgres.NewAccountRepository(p), protector, limits.CodeRequest, limits.WrongCode, limits.AccountWrongCode, key)
	}
	seed := func(t *testing.T, verified bool) (string, string, string) {
		t.Helper()
		subject := uuid.NewString()
		email := subject + "@example.com"
		q := identitysqlc.New(pool)
		if _, err := q.CreateAccount(ctx, identitysqlc.CreateAccountParams{Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test"}); err != nil {
			t.Fatal(err)
		}
		if verified {
			if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET email_verified_at=statement_timestamp() WHERE subject=$1`, subject); err != nil {
				t.Fatal(err)
			}
		}
		digest := sha256.Sum256([]byte(subject))
		session, err := q.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: digest[:]})
		if err != nil {
			t.Fatal(err)
		}
		id := uuid.UUID(session.ID.Bytes).String()
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=NULL WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		return subject, id, email
	}
	material := func(t *testing.T, subject string) (code, id, eventID string) {
		t.Helper()
		var purpose string
		var encrypted outbound.DeliveryMaterial
		if err := pool.QueryRow(ctx, `SELECT challenge.id::text,event.id::text,challenge.purpose,delivery.key_version,delivery.nonce,delivery.ciphertext FROM identity_challenges challenge JOIN identity_challenge_deliveries delivery ON delivery.challenge_id=challenge.id JOIN identity_outbox_events event ON event.challenge_id=challenge.id WHERE challenge.account_subject=$1 AND challenge.purpose='reauthenticate' AND challenge.replaced_at IS NULL AND challenge.consumed_at IS NULL`, subject).Scan(&id, &eventID, &purpose, &encrypted.KeyVersion, &encrypted.Nonce, &encrypted.Ciphertext); err != nil {
			t.Fatal(err)
		}
		_, code, err = protector.Open(id, purpose, subject, encrypted)
		if err != nil || purpose != "reauthenticate" {
			t.Fatalf("wrong protected purpose: %v", err)
		}
		return
	}
	request := func(t *testing.T, subject, id, source string) string {
		t.Helper()
		if err := service(pool).RequestReauthenticationCode(ctx, inbound.RequestReauthenticationCodeInput{Subject: subject, SessionID: id, Source: source}); err != nil {
			t.Fatal(err)
		}
		code, _, _ := material(t, subject)
		return code
	}
	prove := func(p *pgxpool.Pool, subject, id, source, code string) (inbound.ReauthenticateWithCodeResult, error) {
		return service(p).ReauthenticateWithCode(ctx, inbound.ReauthenticateWithCodeInput{Subject: subject, SessionID: id, Source: source, Code: code})
	}
	assertClosed := func(t *testing.T, subject, id string) {
		t.Helper()
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
	}
	t.Run("verified code reaches Mailpit and opens one window", func(t *testing.T) {
		subject, id, email := seed(t, true)
		code := request(t, subject, id, "192.0.2.151")
		_, challengeID, eventID := material(t, subject)
		deliveries := identitypostgres.NewDeliveryRepository(pool)
		mailFailure := errors.New("mail server unavailable")
		if err := deliveries.WithCurrentDelivery(ctx, challengeID, "reauthenticate", func(context.Context, outbound.CurrentDelivery) error { return mailFailure }); !errors.Is(err, mailFailure) {
			t.Fatalf("failed send: %v", err)
		}
		if err := deliveries.PurgeTerminal(ctx); err != nil {
			t.Fatal(err)
		}
		var pending int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenge_deliveries WHERE challenge_id=$1`, challengeID).Scan(&pending); err != nil || pending != 1 {
			t.Fatalf("cleanup lost retry material: count=%d err=%v", pending, err)
		}
		mailpit, err := testcontainers.Run(ctx, "axllent/mailpit:v1.31.1", testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"), testcontainers.WithWaitStrategy(wait.ForHTTP("/api/v1/info").WithPort("8025/tcp")))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = testcontainers.TerminateContainer(mailpit) }()
		host, err := mailpit.Host(ctx)
		if err != nil {
			t.Fatal(err)
		}
		smtp, err := mailpit.MappedPort(ctx, "1025/tcp")
		if err != nil {
			t.Fatal(err)
		}
		web, err := mailpit.MappedPort(ctx, "8025/tcp")
		if err != nil {
			t.Fatal(err)
		}
		sender, err := identityemail.NewMailpitSender(net.JoinHostPort(host, smtp.Port()), "no-reply@flowspace.local")
		if err != nil {
			t.Fatal(err)
		}
		worker, err := identityevent.NewEmailWorker("127.0.0.1:1", "reauthentication-email", "reauthentication-test", identitypostgres.NewDeliveryRepository(pool), protector, sender, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		defer worker.Close()
		payload, err := proto.Marshal(&identityv1.EmailDeliveryRequested{EventId: eventID, ChallengeId: challengeID, Purpose: "reauthenticate"})
		if err != nil {
			t.Fatal(err)
		}
		header, err := (&sr.ConfluentHeader{}).AppendEncode(nil, 1, []int{0})
		if err != nil {
			t.Fatal(err)
		}
		record := &kgo.Record{Key: []byte(eventID), Value: append(header, payload...)}
		if err := worker.HandleRecord(ctx, record); err != nil {
			t.Fatal(err)
		}
		incoming, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, web.Port())+"/api/v1/message/latest/raw", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(incoming)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != 200 || !bytes.Contains(raw, []byte(code)) || !bytes.Contains(raw, []byte(email)) || !bytes.Contains(raw, []byte("Flowspace re-authentication code")) {
			t.Fatal("re-authentication code did not reach Mailpit")
		}
		if bytes.Contains(record.Value, []byte(code)) || bytes.Contains(record.Value, []byte(email)) {
			t.Fatal("delivery event exposed code or email")
		}
		result, err := prove(pool, subject, id, "192.0.2.151", code)
		if err != nil || result.ExpiresAt.IsZero() {
			t.Fatal(err)
		}
		if err := sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil); err != nil {
			t.Fatal(err)
		}
		if _, err := prove(pool, subject, id, "192.0.2.151", code); !errors.Is(err, app.ErrInvalidReauthenticationCode) {
			t.Fatalf("used code accepted: %v", err)
		}
	})
	t.Run("terminal reauthentication payloads are purged", func(t *testing.T) {
		for _, update := range []string{
			`UPDATE identity_challenges SET replaced_at=statement_timestamp() WHERE id=$1`,
			`UPDATE identity_challenges SET consumed_at=statement_timestamp() WHERE id=$1`,
			`UPDATE identity_challenges SET wrong_guesses=5 WHERE id=$1`,
			`UPDATE identity_challenges SET issued_at=statement_timestamp()-INTERVAL '11 minutes',expires_at=statement_timestamp()-INTERVAL '1 minute' WHERE id=$1`,
			`UPDATE identity_accounts SET email_verified_at=NULL WHERE subject=(SELECT account_subject FROM identity_challenges WHERE id=$1)`,
			`UPDATE identity_accounts SET email_local='changed' WHERE subject=(SELECT account_subject FROM identity_challenges WHERE id=$1)`,
			`UPDATE identity_accounts SET email_verified_at=NULL,retired_at=statement_timestamp() WHERE subject=(SELECT account_subject FROM identity_challenges WHERE id=$1)`,
		} {
			subject, id, _ := seed(t, true)
			request(t, subject, id, "192.0.2.162")
			_, challengeID, _ := material(t, subject)
			if _, err := pool.Exec(ctx, update, challengeID); err != nil {
				t.Fatal(err)
			}
			if err := identitypostgres.NewDeliveryRepository(pool).PurgeTerminal(ctx); err != nil {
				t.Fatal(err)
			}
			var remaining int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenge_deliveries WHERE challenge_id=$1`, challengeID).Scan(&remaining); err != nil || remaining != 0 {
				t.Fatalf("terminal material remains: count=%d err=%v update=%s", remaining, err, update)
			}
		}
	})
	t.Run("unverified account receives no code", func(t *testing.T) {
		subject, id, _ := seed(t, false)
		err := service(pool).RequestReauthenticationCode(ctx, inbound.RequestReauthenticationCodeInput{Subject: subject, SessionID: id, Source: "192.0.2.152"})
		if !errors.Is(err, app.ErrReauthenticationEmailUnverified) {
			t.Fatal(err)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE account_subject=$1`, subject).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unverified challenge count=%d err=%v", count, err)
		}
		assertClosed(t, subject, id)
	})
	t.Run("concurrent sessions consume one code once across replicas", func(t *testing.T) {
		subject, id, _ := seed(t, true)
		code := request(t, subject, id, "192.0.2.153")
		digest := sha256.Sum256([]byte(id))
		second, err := identitysqlc.New(pool).CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: digest[:]})
		if err != nil {
			t.Fatal(err)
		}
		other := uuid.UUID(second.ID.Bytes).String()
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=NULL WHERE id=$1`, other); err != nil {
			t.Fatal(err)
		}
		replica, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		defer replica.Close()
		type outcome struct {
			id     string
			result inbound.ReauthenticateWithCodeResult
			err    error
		}
		outcomes := make(chan outcome, 2)
		var group sync.WaitGroup
		for i, session := range []string{id, other} {
			group.Go(func() {
				p := pool
				if i == 1 {
					p = replica
				}
				result, err := prove(p, subject, session, "192.0.2.153", code)
				outcomes <- outcome{session, result, err}
			})
		}
		group.Wait()
		close(outcomes)
		passed := 0
		for outcome := range outcomes {
			if outcome.err == nil {
				passed++
				if outcome.result.ExpiresAt.IsZero() {
					t.Fatal("missing expiry")
				}
				if err := sensitiveTestChange(ctx, pool, subject, outcome.id, uuid.NewString(), nil); err != nil {
					t.Fatal(err)
				}
			} else {
				if !errors.Is(outcome.err, app.ErrInvalidReauthenticationCode) {
					t.Fatal(outcome.err)
				}
				assertClosed(t, subject, outcome.id)
			}
		}
		if passed != 1 {
			t.Fatalf("successful concurrent proofs=%d", passed)
		}
	})
	t.Run("other-purpose codes cannot reauthenticate", func(t *testing.T) {
		for _, purpose := range []domain.CodePurpose{domain.PurposeVerifyEmail, domain.PurposeClaimAccount, domain.PurposePasswordReset} {
			subject, id, email := seed(t, true)
			code, verifier, _, err := domain.NewChallenge(key, subject, email, purpose, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := identitysqlc.New(pool).CreateChallenge(ctx, identitysqlc.CreateChallengeParams{AccountSubject: subject, Purpose: string(purpose), EmailLocal: subject, EmailDomain: "example.com", CodeVerifier: verifier[:]}); err != nil {
				t.Fatal(err)
			}
			if _, err := prove(pool, subject, id, "192.0.2.154", code); !errors.Is(err, app.ErrInvalidReauthenticationCode) {
				t.Fatalf("accepted %s code: %v", purpose, err)
			}
			assertClosed(t, subject, id)
		}
	})
	t.Run("reauthentication code cannot claim reset or verify", func(t *testing.T) {
		subject, id, email := seed(t, true)
		code := request(t, subject, id, "192.0.2.155")
		repo := identitypostgres.NewAccountRepository(pool)
		limit := func(context.Context, string) error { return nil }
		safe := func(context.Context, string) (bool, error) { return false, nil }
		reset := app.NewPasswordResetService(repo, limit, limit, limit, safe, key)
		if err := reset.ResetPassword(ctx, inbound.ResetPasswordInput{Email: email, Code: code, NewPassword: "a safe password for the fixture", Source: "192.0.2.155"}); !errors.Is(err, app.ErrInvalidPasswordResetCode) {
			t.Fatalf("reset accepted reauthentication code: %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET email_verified_at=NULL WHERE subject=$1`, subject); err != nil {
			t.Fatal(err)
		}
		claim := app.NewAccountClaimService(repo, loginLimitSigner{}, limit, limit, safe, key)
		if _, err := claim.ClaimUnverifiedAccount(ctx, inbound.ClaimAccountInput{Email: email, Code: code, NewPassword: "a safe password for the fixture", Source: "192.0.2.155"}); !errors.Is(err, app.ErrInvalidClaimCode) {
			t.Fatalf("claim accepted reauthentication code: %v", err)
		}
		verification := app.NewEmailVerificationService(repo, limit, limit, key)
		if err := verification.VerifyEmail(ctx, inbound.VerifyEmailInput{Subject: subject, SessionID: id, Code: code, Source: "192.0.2.155"}); !errors.Is(err, app.ErrInvalidVerificationCode) {
			t.Fatalf("verification accepted reauthentication code: %v", err)
		}
		var verified bool
		if err := pool.QueryRow(ctx, `SELECT email_verified_at IS NOT NULL FROM identity_accounts WHERE subject=$1`, subject).Scan(&verified); err != nil || verified {
			t.Fatal("re-authentication code changed verified state")
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET email_verified_at=statement_timestamp() WHERE subject=$1`, subject); err != nil {
			t.Fatal(err)
		}
		if err := verification.VerifyEmail(ctx, inbound.VerifyEmailInput{Subject: subject, SessionID: id, Code: code, Source: "192.0.2.155"}); err != nil {
			t.Fatal("verified account no-op changed")
		}
		if _, err := prove(pool, subject, id, "192.0.2.155", code); err != nil {
			t.Fatalf("other-purpose attempts consumed the code: %v", err)
		}
	})
	t.Run("cooldown preserves challenge and counts source retries", func(t *testing.T) {
		subject, id, _ := seed(t, true)

		oldSubject, oldID, _ := seed(t, false)
		t.Cleanup(func() {
			if _, err := pool.Exec(ctx, `DELETE FROM identity_outbox_events WHERE challenge_id IN (SELECT id FROM identity_challenges WHERE account_subject=$1)`, oldSubject); err != nil {
				t.Error(err)
			}
			if _, err := pool.Exec(ctx, `DELETE FROM identity_challenges WHERE account_subject=$1`, oldSubject); err != nil {
				t.Error(err)
			}
		})
		oldLimit := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
		old := app.NewEmailVerificationCodeService(identitypostgres.NewAccountRepository(pool), protector, oldLimit.CodeRequest, key)
		if err := old.RequestEmailVerificationCode(ctx, inbound.RequestEmailVerificationCodeInput{Subject: oldSubject, SessionID: oldID, Source: "192.0.2.156"}); err != nil {
			t.Fatal(err)
		}
		code := request(t, subject, id, "192.0.2.156")
		_, before, _ := material(t, subject)
		for range 58 {
			if err := service(pool).RequestReauthenticationCode(ctx, inbound.RequestReauthenticationCodeInput{Subject: subject, SessionID: id, Source: "192.0.2.156"}); err != nil {
				t.Fatal(err)
			}
		}
		_, after, _ := material(t, subject)
		if after != before {
			t.Fatal("cooldown replaced challenge")
		}
		replica, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		defer replica.Close()
		if err := service(replica).RequestReauthenticationCode(ctx, inbound.RequestReauthenticationCodeInput{Subject: subject, SessionID: id, Source: "192.0.2.156"}); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("retries escaped shared source limit: %v", err)
		}
		if _, err := prove(replica, subject, id, "192.0.2.156", code); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("replacement preserves other challenge purposes", func(t *testing.T) {
		subject, id, _ := seed(t, true)
		for _, purpose := range []string{"verify-email", "claim-account", "password-reset"} {
			if _, err := pool.Exec(ctx, `INSERT INTO identity_challenges(account_subject,purpose,email_local,email_domain,code_verifier,issued_at,expires_at) VALUES($1,$2,$1,'example.com',$3,statement_timestamp()-INTERVAL '2 minutes',statement_timestamp()+INTERVAL '8 minutes')`, subject, purpose, bytes.Repeat([]byte{1}, 32)); err != nil {
				t.Fatal(err)
			}
		}
		otherPurposes := func() string {
			t.Helper()
			var rows string
			if err := pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(challenge) ORDER BY id)::text FROM identity_challenges challenge WHERE account_subject=$1 AND purpose<>'reauthenticate'`, subject).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			return rows
		}
		before := otherPurposes()
		request(t, subject, id, "192.0.2.164")
		_, previous, _ := material(t, subject)
		if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at=statement_timestamp()-INTERVAL '61 seconds',expires_at=statement_timestamp()+INTERVAL '539 seconds' WHERE id=$1`, previous); err != nil {
			t.Fatal(err)
		}
		request(t, subject, id, "192.0.2.164")
		_, current, _ := material(t, subject)
		var replaced bool
		var pending, live int
		if err := pool.QueryRow(ctx, `SELECT replaced_at IS NOT NULL,(SELECT count(*) FROM identity_challenge_deliveries WHERE challenge_id=$1),(SELECT count(*) FROM identity_challenges WHERE account_subject=$2 AND purpose='reauthenticate' AND replaced_at IS NULL AND consumed_at IS NULL) FROM identity_challenges WHERE id=$1`, previous, subject).Scan(&replaced, &pending, &live); err != nil {
			t.Fatal(err)
		}
		if previous == current || !replaced || pending != 0 || live != 1 || otherPurposes() != before {
			t.Fatalf("replacement lost isolation: replaced=%v pending=%d live=%d", replaced, pending, live)
		}
	})
	t.Run("account issue limit includes all older purposes", func(t *testing.T) {
		subject, id, _ := seed(t, true)
		for i := range 5 {
			purpose := []string{"verify-email", "claim-account", "password-reset"}[i%3]
			if _, err := pool.Exec(ctx, `INSERT INTO identity_challenges(account_subject,purpose,email_local,email_domain,code_verifier,issued_at,expires_at,replaced_at) VALUES($1,$2,$1,'example.com',$3,statement_timestamp()-INTERVAL '2 minutes',statement_timestamp()+INTERVAL '8 minutes',statement_timestamp())`, subject, purpose, bytes.Repeat([]byte{1}, 32)); err != nil {
				t.Fatal(err)
			}
		}
		if err := service(pool).RequestReauthenticationCode(ctx, inbound.RequestReauthenticationCodeInput{Subject: subject, SessionID: id, Source: "192.0.2.161"}); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("shared account issue limit=%v", err)
		}
		assertClosed(t, subject, id)
	})
	t.Run("wrong-guess limits are shared with verification", func(t *testing.T) {
		subject, id, _ := seed(t, true)
		code := request(t, subject, id, "192.0.2.157")
		old := app.NewEmailVerificationService(identitypostgres.NewAccountRepository(pool), app.NewLimitService(identitypostgres.NewLimitRepository(pool)).WrongCode, app.NewLimitService(identitypostgres.NewLimitRepository(pool)).AccountWrongCode, key)
		for range 10 {
			if err := old.VerifyEmail(ctx, inbound.VerifyEmailInput{Subject: subject, SessionID: id, Code: "000000", Source: "192.0.2.157"}); err != nil {
				t.Fatal(err)
			}
		}
		replica, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		defer replica.Close()
		if _, err := prove(replica, subject, id, "192.0.2.157", code); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("shared account guess limit=%v", err)
		}
		assertClosed(t, subject, id)
	})
	t.Run("source guess limit is shared across purposes and accounts", func(t *testing.T) {
		const source = "192.0.2.163"
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(pool))
		old := app.NewEmailVerificationService(identitypostgres.NewAccountRepository(pool), limits.WrongCode, limits.AccountWrongCode, key)
		for i := range 10 {
			subject, id, _ := seed(t, true)
			attempts := 10
			if i == 9 {
				attempts = 9
			}
			for range attempts {
				if err := old.VerifyEmail(ctx, inbound.VerifyEmailInput{Subject: subject, SessionID: id, Code: "000000", Source: source}); err != nil {
					t.Fatal(err)
				}
			}
		}
		subject, id, _ := seed(t, true)
		if _, err := prove(pool, subject, id, source, "000000"); !errors.Is(err, app.ErrInvalidReauthenticationCode) {
			t.Fatalf("attempt 100: %v", err)
		}
		assertClosed(t, subject, id)
		other, otherID, _ := seed(t, true)
		code := request(t, other, otherID, source)
		replica, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		defer replica.Close()
		if _, err := prove(replica, other, otherID, source, code); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("attempt 101 escaped shared source bucket: %v", err)
		}
		assertClosed(t, other, otherID)
	})
	t.Run("limit-store outage fails closed", func(t *testing.T) {
		subject, id, _ := seed(t, true)
		closed, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		closed.Close()
		limits := app.NewLimitService(identitypostgres.NewLimitRepository(closed))
		s := app.NewCodeReauthenticationService(identitypostgres.NewAccountRepository(pool), protector, limits.CodeRequest, limits.WrongCode, limits.AccountWrongCode, key)
		if err := s.RequestReauthenticationCode(ctx, inbound.RequestReauthenticationCodeInput{Subject: subject, SessionID: id, Source: "192.0.2.158"}); !errors.Is(err, app.ErrLimitUnavailable) {
			t.Fatal(err)
		}
		code := request(t, subject, id, "192.0.2.158")
		if _, err := s.ReauthenticateWithCode(ctx, inbound.ReauthenticateWithCodeInput{Subject: subject, SessionID: id, Code: code, Source: "192.0.2.158"}); !errors.Is(err, app.ErrLimitUnavailable) {
			t.Fatal(err)
		}
		assertClosed(t, subject, id)
	})

	t.Run("proof commit is atomic with code consumption", func(t *testing.T) {
		for _, kind := range []string{"success", "cancellation", "expiry"} {
			t.Run(kind, func(t *testing.T) {
				for attempt := range 3 {
					subject, id, _ := seed(t, true)
					code := request(t, subject, id, "192.0.2.160")
					_, challengeID, _ := material(t, subject)
					if kind == "expiry" {
						if _, err := pool.Exec(ctx, `UPDATE identity_challenges SET issued_at=statement_timestamp()-INTERVAL '10 minutes'+INTERVAL '2 seconds',expires_at=statement_timestamp()+INTERVAL '2 seconds' WHERE id=$1`, challengeID); err != nil {
							t.Fatal(err)
						}
					}
					proofCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					delay := &commitDelayTracer{ready: make(chan struct{}), release: make(chan struct{})}
					config := pool.Config().Copy()
					config.ConnConfig.Tracer = delay
					delayed, err := pgxpool.NewWithConfig(proofCtx, config)
					if err != nil {
						cancel()
						t.Fatal(err)
					}
					type outcome struct {
						result inbound.ReauthenticateWithCodeResult
						err    error
					}
					finished := make(chan outcome, 1)
					go func() {
						result, err := service(delayed).ReauthenticateWithCode(proofCtx, inbound.ReauthenticateWithCodeInput{Subject: subject, SessionID: id, Source: "192.0.2.160", Code: code})
						finished <- outcome{result, err}
					}()
					select {
					case <-delay.ready:
					case result := <-finished:
						cancel()
						delayed.Close()
						if kind == "expiry" && errors.Is(result.err, app.ErrInvalidReauthenticationCode) && attempt < 2 {
							continue
						}
						t.Fatalf("proof did not reach commit: %v", result.err)
					case <-proofCtx.Done():
						cancel()
						delayed.Close()
						t.Fatal("proof never reached commit")
					}
					var barrier time.Time
					switch kind {
					case "cancellation":
						cancel()
					case "expiry":
						if _, err := pool.Exec(proofCtx, `SELECT pg_sleep(GREATEST(0,EXTRACT(EPOCH FROM (expires_at-clock_timestamp())))+0.02) FROM identity_challenges WHERE id=$1`, challengeID); err != nil {
							cancel()
							delayed.Close()
							t.Fatal(err)
						}
					case "success":
						if _, err := pool.Exec(proofCtx, `SELECT pg_sleep(0.15)`); err != nil {
							cancel()
							delayed.Close()
							t.Fatal(err)
						}
						if err := pool.QueryRow(proofCtx, `SELECT clock_timestamp()`).Scan(&barrier); err != nil {
							cancel()
							delayed.Close()
							t.Fatal(err)
						}
					}
					close(delay.release)
					result := <-finished
					cancel()
					delayed.Close()
					var missing, consumed bool
					if err := pool.QueryRow(ctx, `SELECT session.reauthenticated_at IS NULL,challenge.consumed_at IS NOT NULL FROM identity_sessions session,identity_challenges challenge WHERE session.id=$1 AND challenge.id=$2`, id, challengeID).Scan(&missing, &consumed); err != nil {
						t.Fatal(err)
					}
					if kind == "success" {
						var proof time.Time
						if err := pool.QueryRow(ctx, `SELECT reauthenticated_at FROM identity_sessions WHERE id=$1`, id).Scan(&proof); err != nil {
							t.Fatal(err)
						}
						if result.err != nil || missing || !consumed || proof.Before(barrier) || !result.result.ExpiresAt.Equal(proof.Add(10*time.Minute)) {
							t.Fatalf("commit proof=%v barrier=%v consumed=%v result=%v err=%v", proof, barrier, consumed, result.result, result.err)
						}
						if err := sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil); err != nil {
							t.Fatal(err)
						}
					} else {
						want := error(context.Canceled)
						if kind == "expiry" {
							want = app.ErrInvalidReauthenticationCode
						}
						if !errors.Is(result.err, want) || !result.result.ExpiresAt.IsZero() || !missing || consumed {
							t.Fatalf("failed commit persisted state: missing=%v consumed=%v result=%v err=%v", missing, consumed, result.result, result.err)
						}
						assertClosed(t, subject, id)
					}
					return
				}
			})
		}
	})
	t.Run("migration rollback deletes reauthentication deliveries only", func(t *testing.T) {
		subject, id, _ := seed(t, true)
		request(t, subject, id, "192.0.2.159")
		_, challengeID, eventID := material(t, subject)
		if _, err := identitysqlc.New(pool).CreateChallenge(ctx, identitysqlc.CreateChallengeParams{AccountSubject: subject, Purpose: "verify-email", EmailLocal: subject, EmailDomain: "example.com", CodeVerifier: bytes.Repeat([]byte{1}, 32)}); err != nil {
			t.Fatal(err)
		}
		if err := goose.DownToContext(ctx, db, ".", 11); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"identity_challenges", "identity_challenge_deliveries", "identity_outbox_events"} {
			column, value := "id", challengeID
			if table == "identity_challenge_deliveries" {
				column = "challenge_id"
			}
			if table == "identity_outbox_events" {
				value = eventID
			}
			var count int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE "+column+"=$1", value).Scan(&count); err != nil || count != 0 {
				t.Fatalf("rollback left %s rows=%d err=%v", table, count, err)
			}
		}
		var other int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_challenges WHERE account_subject=$1 AND purpose='verify-email'`, subject).Scan(&other); err != nil || other != 1 {
			t.Fatal("rollback removed another purpose")
		}
		if err := goose.UpContext(ctx, db, "."); err != nil {
			t.Fatal(err)
		}
		if _, err := identitysqlc.New(pool).CreateChallenge(ctx, identitysqlc.CreateChallengeParams{AccountSubject: subject, Purpose: "reauthenticate", EmailLocal: subject, EmailDomain: "example.com", CodeVerifier: bytes.Repeat([]byte{2}, 32)}); err != nil {
			t.Fatal(err)
		}
	})
}
