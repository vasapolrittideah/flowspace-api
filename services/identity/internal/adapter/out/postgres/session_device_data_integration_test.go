//go:build integration

package postgres_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func testSessionDeviceDataMigration(t *testing.T, db *sql.DB, pool *pgxpool.Pool) {
	ctx := t.Context()
	if err := goose.DownToContext(ctx, db, ".", 13); err != nil {
		t.Fatal(err)
	}
	const subject = "legacy-device-data"
	if _, err := identitysqlc.New(pool).CreateAccount(ctx, identitysqlc.CreateAccountParams{
		Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test",
	}); err != nil {
		t.Fatal(err)
	}
	var id string
	hash := sha256.Sum256([]byte(subject))
	if err := pool.QueryRow(ctx, `INSERT INTO identity_sessions (account_subject, refresh_token_hash, created_at, idle_expires_at, absolute_expires_at)
		VALUES ($1, $2, statement_timestamp() - INTERVAL '1 day', statement_timestamp() + INTERVAL '29 days', statement_timestamp() + INTERVAL '89 days')
		RETURNING id::text`, subject, hash[:]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	var method *string
	var empty, lastUsedIsCreated bool
	if err := pool.QueryRow(ctx, `SELECT sign_in_method, `+noDeviceData+`, last_used_at = created_at
		FROM identity_sessions WHERE id = $1`, id).Scan(&method, &empty, &lastUsedIsCreated); err != nil {
		t.Fatal(err)
	}
	if method != nil || !empty || !lastUsedIsCreated {
		t.Fatalf("legacy session method = %v, no device data = %t, last used at creation = %t", method, empty, lastUsedIsCreated)
	}

	created, err := identitysqlc.New(pool).CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT sign_in_method, `+noDeviceData+`, last_used_at = created_at
		FROM identity_sessions WHERE id = $1`, created.ID).Scan(&method, &empty, &lastUsedIsCreated); err != nil {
		t.Fatal(err)
	}
	if method != nil || !empty || !lastUsedIsCreated {
		t.Fatalf("new session method = %v, no device data = %t, last used at creation = %t", method, empty, lastUsedIsCreated)
	}
}

const noDeviceData = `created_ip_address IS NULL AND created_user_agent IS NULL
	AND last_used_ip_address IS NULL AND last_used_user_agent IS NULL`

func testSessionDeviceDataDeletion(t *testing.T, pool *pgxpool.Pool) {
	ctx := t.Context()
	queries := identitysqlc.New(pool)
	seed := byte(0)
	account := func(t *testing.T) string {
		t.Helper()
		subject := "device-" + uuid.NewString()
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{
			Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test",
		}); err != nil {
			t.Fatal(err)
		}
		return subject
	}
	session := func(t *testing.T, subject string) (string, string) {
		t.Helper()
		seed++
		secret := make([]byte, 32)
		secret[0], secret[1] = 0xd0, seed
		hash := sha256.Sum256(secret)
		created, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: hash[:]})
		if err != nil {
			t.Fatal(err)
		}
		id := uuid.UUID(created.ID.Bytes).String()
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET sign_in_method = 'password',
			created_ip_address = '192.0.2.10', created_user_agent = 'Mozilla/5.0 created',
			last_used_ip_address = '2001:db8::1', last_used_user_agent = 'Mozilla/5.0 last used'
			WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		return id, base64.RawURLEncoding.EncodeToString(secret)
	}
	deleted := func(t *testing.T, id string) bool {
		t.Helper()
		var empty bool
		if err := pool.QueryRow(ctx, `SELECT `+noDeviceData+` FROM identity_sessions WHERE id = $1`, id).Scan(&empty); err != nil {
			t.Fatal(err)
		}
		return empty
	}
	expect := func(t *testing.T, revoked, kept []string) {
		t.Helper()
		for _, id := range revoked {
			if !deleted(t, id) {
				t.Fatalf("session %s kept its device data", id)
			}
		}
		for _, id := range kept {
			var data string
			if err := pool.QueryRow(ctx, `SELECT concat_ws('|', created_ip_address, created_user_agent, last_used_ip_address, last_used_user_agent)
				FROM identity_sessions WHERE id = $1`, id).Scan(&data); err != nil {
				t.Fatal(err)
			}
			if data != "192.0.2.10|Mozilla/5.0 created|2001:db8::1|Mozilla/5.0 last used" {
				t.Fatalf("active session %s device data = %q", id, data)
			}
		}
	}
	accounts := identitypostgres.NewAccountRepository(pool)
	errRollback := errors.New("rollback")

	t.Run("current session logout", func(t *testing.T) {
		subject := account(t)
		current, _ := session(t, subject)
		other, _ := session(t, subject)
		logout := app.NewCurrentSessionLogoutService(identitypostgres.NewSessionRepository(pool))
		if err := logout.LogoutCurrentSession(ctx, inbound.LogoutCurrentSessionInput{Subject: subject, SessionID: current}); err != nil {
			t.Fatal(err)
		}
		expect(t, []string{current}, []string{other})
	})

	t.Run("all session logout", func(t *testing.T) {
		subject := account(t)
		current, _ := session(t, subject)
		other, _ := session(t, subject)
		unrelated, _ := session(t, account(t))
		logout := app.NewAllSessionLogoutService(accounts)
		if err := logout.LogoutAllSessions(ctx, inbound.LogoutAllSessionsInput{Subject: subject, SessionID: current}); err != nil {
			t.Fatal(err)
		}
		expect(t, []string{current, other}, []string{unrelated})
	})

	t.Run("password reset", func(t *testing.T) {
		subject := account(t)
		first, _ := session(t, subject)
		second, _ := session(t, subject)
		unrelated, _ := session(t, account(t))
		if err := accounts.WithinPasswordResetTransaction(ctx, func(tx outbound.PasswordResetTransaction) error {
			return tx.RevokePasswordSessions(ctx, subject)
		}); err != nil {
			t.Fatal(err)
		}
		expect(t, []string{first, second}, []string{unrelated})
	})

	t.Run("account claim", func(t *testing.T) {
		subject := account(t)
		first, _ := session(t, subject)
		unrelated, _ := session(t, account(t))
		if err := accounts.WithinAccountClaimTransaction(ctx, func(tx outbound.AccountClaimTransaction) error {
			_, err := tx.RetireAndRevokeAccount(ctx, subject)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		expect(t, []string{first}, []string{unrelated})
	})

	t.Run("refresh-token replay", func(t *testing.T) {
		subject := account(t)
		replayed, refreshToken := session(t, subject)
		_, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := token.NewSigner(privateKey, "device-key", "urn:flowspace:identity:local", "flowspace-api")
		if err != nil {
			t.Fatal(err)
		}
		refresh := app.NewSessionRefreshService(identitypostgres.NewSessionRefreshRepository(pool), signer)
		if _, err := refresh.RefreshSession(ctx, refreshToken); err != nil {
			t.Fatal(err)
		}
		if deleted(t, replayed) {
			t.Fatal("a successful refresh deleted the device data")
		}
		if _, err := refresh.RefreshSession(ctx, refreshToken); !errors.Is(err, app.ErrUnauthenticatedRefresh) {
			t.Fatalf("replay = %v", err)
		}
		expect(t, []string{replayed}, nil)
	})

	t.Run("a session inserted as revoked stores no device data", func(t *testing.T) {
		subject := account(t)
		hash := sha256.Sum256([]byte(subject))
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO identity_sessions (account_subject, refresh_token_hash, idle_expires_at, absolute_expires_at, revoked_at,
			created_ip_address, created_user_agent, last_used_ip_address, last_used_user_agent)
			VALUES ($1, $2, statement_timestamp() + INTERVAL '30 days', statement_timestamp() + INTERVAL '90 days', statement_timestamp(),
			'192.0.2.10', 'Mozilla/5.0 created', '2001:db8::1', 'Mozilla/5.0 last used')
			RETURNING id::text`, subject, hash[:]).Scan(&id); err != nil {
			t.Fatal(err)
		}
		expect(t, []string{id}, nil)
	})

	t.Run("failed revocation transaction keeps the device data", func(t *testing.T) {
		subject := account(t)
		first, _ := session(t, subject)
		if err := accounts.WithinPasswordResetTransaction(ctx, func(tx outbound.PasswordResetTransaction) error {
			if err := tx.RevokePasswordSessions(ctx, subject); err != nil {
				return err
			}
			return errRollback
		}); !errors.Is(err, errRollback) {
			t.Fatalf("reset transaction = %v", err)
		}
		if err := accounts.WithinAccountClaimTransaction(ctx, func(tx outbound.AccountClaimTransaction) error {
			if _, err := tx.RetireAndRevokeAccount(ctx, subject); err != nil {
				return err
			}
			return errRollback
		}); !errors.Is(err, errRollback) {
			t.Fatalf("claim transaction = %v", err)
		}
		var revoked bool
		if err := pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM identity_sessions WHERE id = $1`, first).Scan(&revoked); err != nil || revoked {
			t.Fatalf("rolled-back session revoked = %t, %v", revoked, err)
		}
		expect(t, nil, []string{first})
	})
	t.Run("expiry cleanup deletes only expired device data", func(t *testing.T) {
		subject := account(t)
		before, _ := session(t, subject)
		at, _ := session(t, subject)
		after, _ := session(t, subject)
		lastUsedOnly, _ := session(t, subject)
		agentOnly, _ := session(t, subject)
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET created_ip_address = NULL, created_user_agent = NULL WHERE id = $1`, lastUsedOnly); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET created_ip_address = NULL, last_used_ip_address = NULL, last_used_user_agent = NULL
			WHERE id = $1`, agentOnly); err != nil {
			t.Fatal(err)
		}
		expire := func(id, expiry string) {
			t.Helper()
			if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET created_at = statement_timestamp() - INTERVAL '2 hours',
				last_used_at = statement_timestamp() - INTERVAL '2 hours', idle_expires_at = `+expiry+` WHERE id = $1`, id); err != nil {
				t.Fatal(err)
			}
		}
		// A session expires at its exact expiry time, so the cleanup that runs at that time deletes its data.
		expire(before, `clock_timestamp() + INTERVAL '3 seconds'`)
		expire(after, `statement_timestamp() - INTERVAL '59 minutes'`)
		expire(lastUsedOnly, `statement_timestamp() - INTERVAL '1 minute'`)
		expire(agentOnly, `statement_timestamp() - INTERVAL '1 minute'`)
		cleanup := identitypostgres.NewSessionRepository(pool)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `UPDATE identity_sessions SET created_at = statement_timestamp() - INTERVAL '2 hours',
			last_used_at = statement_timestamp() - INTERVAL '2 hours', idle_expires_at = transaction_timestamp() WHERE id = $1`, at); err != nil {
			t.Fatal(err)
		}
		if err := identitypostgres.NewSessionRepository(tx).PurgeExpiredDeviceData(ctx); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := cleanup.PurgeExpiredDeviceData(ctx); err != nil {
			t.Fatal(err)
		}
		expect(t, []string{at, after, lastUsedOnly, agentOnly}, []string{before})
		time.Sleep(3100 * time.Millisecond)
		if err := cleanup.PurgeExpiredDeviceData(ctx); err != nil {
			t.Fatal(err)
		}
		expect(t, []string{before}, nil)
	})
}
