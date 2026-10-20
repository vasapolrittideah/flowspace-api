//go:build integration

package postgres_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func testPasswordReauthentication(t *testing.T, pool *pgxpool.Pool, db *sql.DB) {
	ctx := t.Context()
	password := "correct café battery staple"
	hash, err := domain.HashPassword(ctx, password, func(context.Context, string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	seed := func(t *testing.T) (string, string, string) {
		t.Helper()
		subject := uuid.NewString()
		email := subject + "@example.com"
		queries := identitysqlc.New(pool)
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: hash}); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(subject))
		session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: digest[:]})
		if err != nil {
			t.Fatal(err)
		}
		id := uuid.UUID(session.ID.Bytes).String()
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=NULL WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		return subject, id, email
	}
	newService := func(p *pgxpool.Pool) *app.PasswordReauthenticationService {
		return app.NewPasswordReauthenticationService(identitypostgres.NewAccountRepository(p), app.NewLimitService(identitypostgres.NewLimitRepository(p)).PasswordLogin)
	}
	t.Run("proof retry isolation and concurrent replicas", func(t *testing.T) {
		subject, id, _ := seed(t)
		second := sha256.Sum256([]byte(id))
		row, err := identitysqlc.New(pool).CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: second[:]})
		if err != nil {
			t.Fatal(err)
		}
		other := uuid.UUID(row.ID.Bytes).String()
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=NULL WHERE id=$1`, other); err != nil {
			t.Fatal(err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
		replica, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		defer replica.Close()
		input := inbound.ReauthenticateWithPasswordInput{Subject: subject, SessionID: id, Password: "correct cafe\u0301 battery staple", Source: "192.0.2.210"}
		var group sync.WaitGroup
		results := make(chan inbound.ReauthenticateWithPasswordResult, 2)
		failures := make(chan error, 2)
		for _, service := range []*app.PasswordReauthenticationService{newService(pool), newService(replica)} {
			group.Go(func() {
				result, err := service.ReauthenticateWithPassword(ctx, input)
				results <- result
				failures <- err
			})
		}
		group.Wait()
		close(results)
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
		var expiry time.Time
		for result := range results {
			if expiry.IsZero() {
				expiry = result.ExpiresAt
			} else if !expiry.Equal(result.ExpiresAt) {
				t.Fatal("concurrent retry extended window")
			}
		}
		var proof time.Time
		if err := pool.QueryRow(ctx, `SELECT reauthenticated_at FROM identity_sessions WHERE id=$1`, id).Scan(&proof); err != nil || !expiry.Equal(proof.Add(10*time.Minute)) {
			t.Fatalf("proof end mismatch: %v", err)
		}
		if err := sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil); err != nil {
			t.Fatal(err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, other, uuid.NewString(), nil))
		input.Password = "wrong password"
		if _, err := newService(pool).ReauthenticateWithPassword(ctx, input); !errors.Is(err, app.ErrInvalidCredentials) {
			t.Fatalf("retry skipped proof: %v", err)
		}
	})
	t.Run("password login and reauthentication share limits across replicas", func(t *testing.T) {
		subject, id, email := seed(t)
		replica, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		defer replica.Close()
		login := app.NewPasswordLoginService(identitypostgres.NewAccountRepository(replica), loginLimitSigner{}, app.NewLimitService(identitypostgres.NewLimitRepository(replica)).PasswordLogin)
		for range 9 {
			if _, err := login.CreatePasswordSession(ctx, inbound.CreatePasswordSessionInput{Email: email, Password: "wrong password", Source: "192.0.2.211"}); !errors.Is(err, app.ErrInvalidCredentials) {
				t.Fatal(err)
			}
		}
		input := inbound.ReauthenticateWithPasswordInput{Subject: subject, SessionID: id, Password: "wrong password", Source: "192.0.2.212"}
		if _, err := newService(pool).ReauthenticateWithPassword(ctx, input); !errors.Is(err, app.ErrInvalidCredentials) {
			t.Fatal(err)
		}
		input.Password = password
		if _, err := newService(replica).ReauthenticateWithPassword(ctx, input); !errors.Is(err, app.ErrRateLimited) {
			t.Fatalf("shared email limit=%v", err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
		if _, err := pool.Exec(ctx, `UPDATE identity_limit_counters SET window_start=window_start-INTERVAL '2 hours' WHERE action='password-login' AND counter_key IN ('192.0.2.211','192.0.2.212',encode(sha256($1::bytea),'hex'))`, []byte(email)); err != nil {
			t.Fatal(err)
		}
		if _, err := newService(pool).ReauthenticateWithPassword(ctx, input); err != nil {
			t.Fatalf("failed attempts locked session/account: %v", err)
		}
	})
	t.Run("no password and inactive sessions keep the window closed", func(t *testing.T) {
		subject, id, _ := seed(t)
		if _, err := pool.Exec(ctx, `UPDATE identity_accounts SET password_hash=NULL WHERE subject=$1`, subject); err != nil {
			t.Fatal(err)
		}
		input := inbound.ReauthenticateWithPasswordInput{Subject: subject, SessionID: id, Password: password, Source: "192.0.2.213"}
		if _, err := newService(pool).ReauthenticateWithPassword(ctx, input); !errors.Is(err, app.ErrInvalidCredentials) {
			t.Fatal(err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := newService(pool).ReauthenticateWithPassword(ctx, input); !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatal(err)
		}
	})

	t.Run("failure after proof write rolls back", func(t *testing.T) {
		for _, expire := range []bool{false, true} {
			name := "cancellation"
			if expire {
				name = "expiry"
			}
			t.Run(name, func(t *testing.T) {
				for attempt := range 3 {
					subject, id, _ := seed(t)
					if expire {
						if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET idle_expires_at=clock_timestamp()+INTERVAL '2 seconds' WHERE id=$1`, id); err != nil {
							t.Fatal(err)
						}
					}
					request, cancel := context.WithTimeout(ctx, 10*time.Second)
					delay := &commitDelayTracer{ready: make(chan struct{}), release: make(chan struct{})}
					config := pool.Config().Copy()
					config.ConnConfig.Tracer = delay
					delayed, err := pgxpool.NewWithConfig(request, config)
					if err != nil {
						cancel()
						t.Fatal(err)
					}
					type response struct {
						result inbound.ReauthenticateWithPasswordResult
						err    error
					}
					finished := make(chan response, 1)
					go func() {
						result, err := newService(delayed).ReauthenticateWithPassword(request, inbound.ReauthenticateWithPasswordInput{Subject: subject, SessionID: id, Password: password, Source: "192.0.2.216"})
						finished <- response{result, err}
					}()
					select {
					case <-delay.ready:
					case result := <-finished:
						cancel()
						delayed.Close()
						if expire && errors.Is(result.err, outbound.ErrUnauthenticated) && attempt < 2 {
							continue
						}
						t.Fatalf("request did not reach commit: %v", result.err)
					case <-request.Done():
						cancel()
						delayed.Close()
						t.Fatal("request never wrote proof")
					}
					want := error(context.Canceled)
					if expire {
						if _, err := pool.Exec(request, `SELECT pg_sleep(GREATEST(0,EXTRACT(EPOCH FROM (idle_expires_at-clock_timestamp())))+0.02) FROM identity_sessions WHERE id=$1`, id); err != nil {
							cancel()
							delayed.Close()
							t.Fatal(err)
						}
						want = outbound.ErrUnauthenticated
					} else {
						cancel()
					}
					close(delay.release)
					result := <-finished
					cancel()
					delayed.Close()
					if !errors.Is(result.err, want) || !result.result.ExpiresAt.IsZero() {
						t.Fatalf("failed commit response=%v err=%v want=%v", result.result, result.err, want)
					}
					var missing bool
					if err := pool.QueryRow(ctx, `SELECT reauthenticated_at IS NULL FROM identity_sessions WHERE id=$1`, id).Scan(&missing); err != nil || !missing {
						t.Fatalf("failed proof persisted: %v", err)
					}
					if expire {
						assertRevokedReauthenticationWindow(t, pool, subject, id)
					} else {
						requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
					}
					return
				}
			})
		}
	})

	t.Run("successful delayed commit starts the full window", func(t *testing.T) {
		subject, id, _ := seed(t)
		request, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		delay := &commitDelayTracer{ready: make(chan struct{}), release: make(chan struct{})}
		config := pool.Config().Copy()
		config.ConnConfig.Tracer = delay
		delayed, err := pgxpool.NewWithConfig(request, config)
		if err != nil {
			t.Fatal(err)
		}
		defer delayed.Close()
		type response struct {
			result inbound.ReauthenticateWithPasswordResult
			err    error
		}
		finished := make(chan response, 1)
		go func() {
			result, err := newService(delayed).ReauthenticateWithPassword(request, inbound.ReauthenticateWithPasswordInput{Subject: subject, SessionID: id, Password: password, Source: "192.0.2.217"})
			finished <- response{result, err}
		}()
		select {
		case <-delay.ready:
		case result := <-finished:
			t.Fatalf("did not reach commit: %v", result.err)
		case <-request.Done():
			t.Fatal(request.Err())
		}
		var afterDelay time.Time
		if _, err := pool.Exec(request, `SELECT pg_sleep(0.25)`); err != nil {
			cancel()
			t.Fatal(err)
		}
		if err := pool.QueryRow(request, `SELECT clock_timestamp()`).Scan(&afterDelay); err != nil {
			cancel()
			t.Fatal(err)
		}
		close(delay.release)
		result := <-finished
		if result.err != nil {
			t.Fatal(result.err)
		}
		var proof time.Time
		if err := pool.QueryRow(ctx, `SELECT reauthenticated_at FROM identity_sessions WHERE id=$1`, id).Scan(&proof); err != nil {
			t.Fatal(err)
		}
		if proof.Before(afterDelay) || !result.result.ExpiresAt.Equal(proof.Add(10*time.Minute)) {
			t.Fatalf("window started before commit: proof=%v barrier=%v expiry=%v", proof, afterDelay, result.result.ExpiresAt)
		}
		if err := sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("unavailable limit store fails closed", func(t *testing.T) {
		subject, id, _ := seed(t)
		closed, err := pgxpool.NewWithConfig(ctx, pool.Config().Copy())
		if err != nil {
			t.Fatal(err)
		}
		closed.Close()
		service := app.NewPasswordReauthenticationService(identitypostgres.NewAccountRepository(pool), app.NewLimitService(identitypostgres.NewLimitRepository(closed)).PasswordLogin)
		if result, err := service.ReauthenticateWithPassword(ctx, inbound.ReauthenticateWithPasswordInput{Subject: subject, SessionID: id, Password: password, Source: "192.0.2.214"}); !errors.Is(err, app.ErrLimitUnavailable) || !result.ExpiresAt.IsZero() {
			t.Fatalf("limit outage=%v", err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
	})
	t.Run("commit-time migration preserves proof through rollback", func(t *testing.T) {
		subject, id, _ := seed(t)
		var before, after time.Time
		if err := pool.QueryRow(ctx, `UPDATE identity_sessions SET reauthenticated_at=clock_timestamp()-INTERVAL '11 minutes' WHERE id=$1 RETURNING reauthenticated_at`, id).Scan(&before); err != nil {
			t.Fatal(err)
		}
		if err := goose.DownToContext(ctx, db, ".", 10); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT reauthenticated_at FROM identity_sessions WHERE id=$1`, id).Scan(&after); err != nil || !after.Equal(before) {
			t.Fatalf("rollback changed proof: %v", err)
		}
		if err := goose.UpContext(ctx, db, "."); err != nil {
			t.Fatal(err)
		}
		result, err := newService(pool).ReauthenticateWithPassword(ctx, inbound.ReauthenticateWithPasswordInput{Subject: subject, SessionID: id, Password: password, Source: "192.0.2.218"})
		if err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT reauthenticated_at FROM identity_sessions WHERE id=$1`, id).Scan(&after); err != nil || !after.After(before) || !result.ExpiresAt.Equal(after.Add(10*time.Minute)) {
			t.Fatalf("migration did not restore commit proof: %v", err)
		}
	})
}

type loginLimitSigner struct{}

func (loginLimitSigner) Sign(outbound.AccessTokenClaims) (string, error) { return "unused", nil }
