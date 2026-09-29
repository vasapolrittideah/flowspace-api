//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/peer"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	deliverycrypto "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

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
