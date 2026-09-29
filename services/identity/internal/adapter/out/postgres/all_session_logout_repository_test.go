//go:build integration

package postgres_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

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
