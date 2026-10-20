//go:build integration

package postgres_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	identitypostgres "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	identitysqlc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type commitDelayKey struct{}

type commitDelayTracer struct {
	ready   chan struct{}
	release chan struct{}
}

func (*commitDelayTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "GetTransactionCheckTime") || strings.Contains(data.SQL, "QueueSessionReauthenticationCheck") {
		return context.WithValue(ctx, commitDelayKey{}, true)
	}
	return ctx
}

func (d *commitDelayTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if active, _ := ctx.Value(commitDelayKey{}).(bool); active {
		select {
		case d.ready <- struct{}{}:
		case <-ctx.Done():
			return
		}
		select {
		case <-d.release:
		case <-ctx.Done():
		}
	}
}

// sensitiveTestChange creates an account only through the guarded transaction.
func sensitiveTestChange(ctx context.Context, pool *pgxpool.Pool, subject, sessionID, marker string, work func() error) error {
	err := identitypostgres.NewAccountRepository(pool).WithinTransaction(ctx, func(tx outbound.AccountTransaction) error {
		guarded, ok := tx.(outbound.ReauthenticationTransaction)
		if !ok {
			return errors.New("transaction has no re-authentication guard")
		}
		if err := app.RequireReauthentication(ctx, guarded, subject, sessionID); err != nil {
			return err
		}
		if err := tx.CreateAccount(ctx, marker, marker+"@example.com", "$argon2id$test"); err != nil {
			return err
		}
		if work != nil {
			return work()
		}
		return nil
	})
	return identityhttp.ReauthenticationRPCError(err)
}

func requireReauthenticationError(t *testing.T, err error) {
	t.Helper()
	result := status.Convert(err)
	if result.Code() != codes.PermissionDenied || len(result.Details()) != 1 {
		t.Fatalf("guard error = %v", err)
	}
	detail, ok := result.Details()[0].(*errdetails.ErrorInfo)
	if !ok || detail.GetReason() != "REAUTHENTICATION_REQUIRED" || detail.GetDomain() != "identity.flowspace" {
		t.Fatalf("guard detail = %v", detail)
	}
}

func assertInitialReauthenticationWindow(t *testing.T, pool *pgxpool.Pool, subject string) {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT id::text, created_at, reauthenticated_at FROM identity_sessions WHERE account_subject=$1 AND revoked_at IS NULL`, subject)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		var created, authenticated time.Time
		if err := rows.Scan(&id, &created, &authenticated); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if !created.Equal(authenticated) {
			rows.Close()
			t.Fatal("initial proof time differs from session creation")
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("login created no session")
	}
	for _, id := range ids {
		if err := sensitiveTestChange(t.Context(), pool, subject, id, uuid.NewString(), nil); err != nil {
			t.Fatalf("fresh login guard = %v", err)
		}
	}
}

func assertRevokedReauthenticationWindows(t *testing.T, pool *pgxpool.Pool, subject string) {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT id::text FROM identity_sessions WHERE account_subject=$1`, subject)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("no sessions to check")
	}
	for _, id := range ids {
		assertRevokedReauthenticationWindow(t, pool, subject, id)
	}
}

func assertRevokedReauthenticationWindow(t *testing.T, pool *pgxpool.Pool, subject, id string) {
	t.Helper()
	if err := sensitiveTestChange(t.Context(), pool, subject, id, uuid.NewString(), nil); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("revoked window = %v", err)
	}
}

func testReauthenticationMigration(t *testing.T, db *sql.DB, pool *pgxpool.Pool) {
	ctx := t.Context()
	if err := goose.DownToContext(ctx, db, ".", 9); err != nil {
		t.Fatal(err)
	}
	const subject = "legacy-reauthentication"
	if _, err := identitysqlc.New(pool).CreateAccount(ctx, identitysqlc.CreateAccountParams{Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test"}); err != nil {
		t.Fatal(err)
	}
	var id string
	hash := sha256.Sum256([]byte(subject))
	if err := pool.QueryRow(ctx, `INSERT INTO identity_sessions(account_subject,refresh_token_hash,idle_expires_at,absolute_expires_at) VALUES ($1,$2,statement_timestamp()+INTERVAL '30 days',statement_timestamp()+INTERVAL '90 days') RETURNING id::text`, subject, hash[:]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		t.Fatal(err)
	}
	var missing bool
	if err := pool.QueryRow(ctx, `SELECT reauthenticated_at IS NULL FROM identity_sessions WHERE id=$1`, id).Scan(&missing); err != nil || !missing {
		t.Fatalf("legacy proof is not NULL: %v", err)
	}
	requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
}

func testReauthenticationGuard(t *testing.T, pool *pgxpool.Pool) {
	ctx := t.Context()
	queries := identitysqlc.New(pool)
	seed := func(t *testing.T) (string, string, []byte) {
		t.Helper()
		subject := uuid.NewString()
		if _, err := queries.CreateAccount(ctx, identitysqlc.CreateAccountParams{Subject: subject, EmailLocal: subject, EmailDomain: "example.com", PasswordHash: "$argon2id$test"}); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(subject))
		session, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: hash[:]})
		if err != nil {
			t.Fatal(err)
		}
		return subject, uuid.UUID(session.ID.Bytes).String(), hash[:]
	}
	t.Run("open window commits the sensitive change", func(t *testing.T) {
		subject, id, _ := seed(t)
		assertInitialReauthenticationWindow(t, pool, subject)
		marker := uuid.NewString()
		if err := sensitiveTestChange(ctx, pool, subject, id, marker, nil); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE subject=$1`, marker).Scan(&count); err != nil || count != 1 {
			t.Fatalf("successful change did not persist: %d, %v", count, err)
		}
	})
	t.Run("closed and absent windows change nothing", func(t *testing.T) {
		for _, proof := range []string{"NULL", "clock_timestamp()-INTERVAL '10 minutes'"} {
			subject, id, _ := seed(t)
			if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=`+proof+` WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			marker := uuid.NewString()
			requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, marker, nil))
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE subject=$1`, marker).Scan(&count); err != nil || count != 0 {
				t.Fatalf("closed window changed state: %d, %v", count, err)
			}
		}
	})
	t.Run("refresh preserves the closed window", func(t *testing.T) {
		subject, id, hash := seed(t)
		var before, after time.Time
		if err := pool.QueryRow(ctx, `UPDATE identity_sessions SET reauthenticated_at=clock_timestamp()-INTERVAL '11 minutes' WHERE id=$1 RETURNING reauthenticated_at`, id).Scan(&before); err != nil {
			t.Fatal(err)
		}
		next := sha256.Sum256(hash)
		if _, err := identitypostgres.NewSessionRefreshRepository(pool).Rotate(ctx, hash, next[:], func(outbound.RefreshSessionRecord) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT reauthenticated_at FROM identity_sessions WHERE id=$1`, id).Scan(&after); err != nil || !after.Equal(before) {
			t.Fatalf("refresh moved proof time: %v", err)
		}
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil))
	})
	t.Run("commit after the final check crosses expiry and rolls back", func(t *testing.T) {
		for attempt := range 3 {
			subject, id, _ := seed(t)
			if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=clock_timestamp()-INTERVAL '10 minutes'+INTERVAL '2 seconds' WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			request, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			delay := &commitDelayTracer{ready: make(chan struct{}), release: make(chan struct{})}
			configuration := pool.Config().Copy()
			configuration.ConnConfig.Tracer = delay
			delayedPool, err := pgxpool.NewWithConfig(request, configuration)
			if err != nil {
				t.Fatal(err)
			}
			defer delayedPool.Close()
			marker := uuid.NewString()
			result := make(chan error, 1)
			go func() { result <- sensitiveTestChange(request, delayedPool, subject, id, marker, nil) }()
			select {
			case err := <-result:
				cancel()
				if status.Code(err) == codes.PermissionDenied && attempt < 2 {
					continue
				}
				t.Fatalf("change never reached the final check: %v", err)
			case <-delay.ready:
			case <-request.Done():
				t.Fatal(request.Err())
			}
			if _, err := pool.Exec(request, `SELECT pg_sleep(GREATEST(0,EXTRACT(EPOCH FROM (reauthenticated_at+INTERVAL '10 minutes'-clock_timestamp())))+0.02) FROM identity_sessions WHERE id=$1`, id); err != nil {
				cancel()
				t.Fatal(err)
			}
			close(delay.release)
			requireReauthenticationError(t, <-result)
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE subject=$1`, marker).Scan(&count); err != nil || count != 0 {
				t.Fatalf("late commit changed state: %d, %v", count, err)
			}
			return
		}
	})
	t.Run("two sessions of one subject keep separate windows", func(t *testing.T) {
		subject, openID, _ := seed(t)
		hash := sha256.Sum256([]byte(openID))
		closed, err := queries.CreateSession(ctx, identitysqlc.CreateSessionParams{AccountSubject: subject, RefreshTokenHash: hash[:]})
		if err != nil {
			t.Fatal(err)
		}
		closedID := uuid.UUID(closed.ID.Bytes).String()
		if _, err := pool.Exec(ctx, `UPDATE identity_sessions SET reauthenticated_at=NULL WHERE id=$1`, closedID); err != nil {
			t.Fatal(err)
		}
		if err := sensitiveTestChange(ctx, pool, subject, openID, uuid.NewString(), nil); err != nil {
			t.Fatal(err)
		}
		marker := uuid.NewString()
		requireReauthenticationError(t, sensitiveTestChange(ctx, pool, subject, closedID, marker, nil))
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE subject=$1`, marker).Scan(&count); err != nil || count != 0 {
			t.Fatalf("closed session changed state: %d, %v", count, err)
		}
	})
	t.Run("inactive and mismatched sessions fail closed", func(t *testing.T) {
		for _, update := range []string{
			`UPDATE identity_sessions SET revoked_at=clock_timestamp() WHERE id=$1`,
			`UPDATE identity_sessions SET idle_expires_at=created_at+INTERVAL '1 microsecond' WHERE id=$1`,
			`UPDATE identity_sessions SET idle_expires_at=created_at+INTERVAL '1 microsecond',absolute_expires_at=created_at+INTERVAL '1 microsecond' WHERE id=$1`,
		} {
			subject, id, _ := seed(t)
			if _, err := pool.Exec(ctx, update, id); err != nil {
				t.Fatal(err)
			}
			if err := sensitiveTestChange(ctx, pool, subject, id, uuid.NewString(), nil); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("inactive session=%v", err)
			}
		}
		subject, id, _ := seed(t)
		for _, pair := range [][2]string{{"other-subject", id}, {subject, uuid.NewString()}, {subject, "invalid-uuid"}} {
			if err := sensitiveTestChange(ctx, pool, pair[0], pair[1], uuid.NewString(), nil); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("wrong identity=%v", err)
			}
		}
	})
	t.Run("work errors and cancellations roll back the sensitive change", func(t *testing.T) {
		for _, canceled := range []bool{false, true} {
			subject, id, _ := seed(t)
			marker := uuid.NewString()
			request, cancel := context.WithCancel(ctx)
			err := sensitiveTestChange(request, pool, subject, id, marker, func() error {
				if canceled {
					cancel()
					return nil
				}
				return errors.New("private database error")
			})
			cancel()
			want := codes.Unavailable
			if canceled {
				want = codes.Canceled
			}
			if status.Code(err) != want {
				t.Fatalf("failed change = %v, want %v", err, want)
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity_accounts WHERE subject=$1`, marker).Scan(&count); err != nil || count != 0 {
				t.Fatalf("failed work changed state: %d, %v", count, err)
			}
		}
	})
}
