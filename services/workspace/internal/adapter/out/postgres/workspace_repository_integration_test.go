//go:build integration

package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/db/migrations"
	workspacesqlc "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

func TestWorkspaceRepository(t *testing.T) {
	ctx := context.Background()
	container, err := postgrescontainer.Run(
		ctx,
		"postgres:18-alpine",
		postgrescontainer.WithDatabase("workspace"),
		postgrescontainer.WithUsername("workspace"),
		postgrescontainer.WithPassword("workspace"),
		postgrescontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Error(err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	applyMigrations(t, ctx, dsn)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	repository := NewWorkspaceRepository(pool)

	t.Run("creates a workspace and owner atomically", func(t *testing.T) {
		created, err := createWorkspace(ctx, repository, "subject-create", "create-key", "Platform")
		if err != nil {
			t.Fatal(err)
		}
		if created.ID == "" || created.Name != "Platform" || created.CreatedAt.IsZero() {
			t.Fatalf("unexpected workspace: %+v", created)
		}

		var role string
		err = pool.QueryRow(ctx, `
			SELECT role
			FROM workspace_memberships
			WHERE workspace_id = $1 AND subject = $2
		`, created.ID, "subject-create").Scan(&role)
		if err != nil {
			t.Fatal(err)
		}
		if role != "owner" {
			t.Fatalf("role = %q, want owner", role)
		}

		var retryWorkspaceID, retryName string
		var requestHash []byte
		var retryCreatedAt, completedAt, expiresAt time.Time
		err = pool.QueryRow(ctx, `
			SELECT request_hash, workspace_id::text, workspace_name, workspace_created_at, completed_at, expires_at
			FROM workspace_creations
			WHERE subject = $1 AND idempotency_key = $2
		`, "subject-create", "create-key").Scan(
			&requestHash,
			&retryWorkspaceID,
			&retryName,
			&retryCreatedAt,
			&completedAt,
			&expiresAt,
		)
		if err != nil {
			t.Fatal(err)
		}
		wantHash := sha256.Sum256([]byte("Platform"))
		if !bytes.Equal(requestHash, wantHash[:]) {
			t.Fatalf("request hash = %x, want %x", requestHash, wantHash)
		}
		if retryWorkspaceID != created.ID || retryName != created.Name || !retryCreatedAt.Equal(created.CreatedAt) {
			t.Fatalf("retry result = %q, %q, %s; want %+v", retryWorkspaceID, retryName, retryCreatedAt, created)
		}
		if got := expiresAt.Sub(completedAt); got != 24*time.Hour {
			t.Fatalf("retry window = %s, want 24h", got)
		}
		_, err = pool.Exec(ctx, `
			INSERT INTO workspace_memberships (workspace_id, subject, role)
			VALUES ($1, $2, 'viewer')
		`, created.ID, "subject-create")
		if err == nil {
			t.Fatal("duplicate membership insert succeeded")
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO workspace_memberships (workspace_id, subject, role)
			VALUES ($1, $2, 'owner')
		`, created.ID, "second-owner")
		if err == nil {
			t.Fatal("second owner insert succeeded")
		}
	})

	t.Run("replays the same idempotent create", func(t *testing.T) {
		first, err := createWorkspace(ctx, repository, "subject-replay", "replay-key", "Replay")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE workspaces SET name = 'Renamed Later' WHERE id = $1`, first.ID); err != nil {
			t.Fatal(err)
		}
		second, err := createWorkspace(ctx, repository, "subject-replay", "replay-key", "Replay")
		if err != nil {
			t.Fatal(err)
		}
		if second != first {
			t.Fatalf("replay = %+v, want %+v", second, first)
		}

		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspaces WHERE id = $1`, first.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("workspace count = %d, want 1", count)
		}

		other, err := createWorkspace(ctx, repository, "another-subject", "replay-key", "Replay")
		if err != nil {
			t.Fatal(err)
		}
		if other.ID == first.ID {
			t.Fatal("idempotency key was not scoped to the subject")
		}

		differentKey, err := createWorkspace(ctx, repository, "subject-replay", "REPLAY-KEY", "Replay")
		if err != nil {
			t.Fatal(err)
		}
		if differentKey.ID == first.ID {
			t.Fatal("idempotency key comparison ignored byte differences")
		}
	})

	t.Run("replays a committed create after reopening the pool", func(t *testing.T) {
		firstPool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		first, createErr := createWorkspace(ctx, NewWorkspaceRepository(firstPool), "subject-reopen", "reopen-key", "Durable")
		firstPool.Close()
		if createErr != nil {
			t.Fatal(createErr)
		}

		reopenedPool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(reopenedPool.Close)
		reopenedRepository := NewWorkspaceRepository(reopenedPool)
		second, err := createWorkspace(ctx, reopenedRepository, "subject-reopen", "reopen-key", "Durable")
		if err != nil {
			t.Fatal(err)
		}
		if second != first {
			t.Fatalf("replay = %+v, want %+v", second, first)
		}
		read, err := reopenedRepository.GetWorkspace(ctx, "subject-reopen", first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if read != first {
			t.Fatalf("read workspace = %+v, want %+v", read, first)
		}
	})

	t.Run("rejects an idempotency key reused with another request", func(t *testing.T) {
		if _, err := createWorkspace(ctx, repository, "subject-conflict", "conflict-key", "Original"); err != nil {
			t.Fatal(err)
		}
		if _, err := createWorkspace(ctx, repository, "subject-conflict", "conflict-key", "Changed"); !errors.Is(err, domain.ErrIdempotencyConflict) {
			t.Fatalf("error = %v, want ErrIdempotencyConflict", err)
		}
	})

	t.Run("rejects a create already in progress", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		locked, err := workspacesqlc.New(tx).TryCreateWorkspaceLock(ctx, createWorkspaceLockID("subject-in-flight", "in-flight-key"))
		if err != nil {
			t.Fatal(err)
		}
		if !locked {
			t.Fatal("failed to acquire setup lock")
		}

		if _, err := createWorkspace(ctx, repository, "subject-in-flight", "in-flight-key", "In Flight"); !errors.Is(err, domain.ErrCreateInProgress) {
			t.Fatalf("error = %v, want ErrCreateInProgress", err)
		}
	})

	t.Run("allows only one concurrent create for the same key", func(t *testing.T) {
		blocker, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = blocker.Rollback(ctx) }()

		if _, err := blocker.Exec(ctx, `LOCK TABLE workspaces IN ACCESS EXCLUSIVE MODE`); err != nil {
			t.Fatal(err)
		}

		type createResult struct {
			workspace domain.Workspace
			err       error
		}
		results := make(chan createResult, 2)
		for range 2 {
			go func() {
				workspace, createErr := createWorkspace(ctx, repository, "subject-concurrent", "concurrent-key", "Concurrent")
				results <- createResult{workspace: workspace, err: createErr}
			}()
		}

		var overlap createResult
		select {
		case overlap = <-results:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent create did not return")
		}
		if !errors.Is(overlap.err, domain.ErrCreateInProgress) {
			t.Fatalf("overlap error = %v, want ErrCreateInProgress", overlap.err)
		}
		if err := blocker.Commit(ctx); err != nil {
			t.Fatal(err)
		}

		var created createResult
		select {
		case created = <-results:
		case <-time.After(5 * time.Second):
			t.Fatal("winning create did not return")
		}
		if created.err != nil {
			t.Fatal(created.err)
		}
		if created.workspace.ID == "" {
			t.Fatal("winning create returned no workspace ID")
		}

		var workspaces, owners, retries int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspaces WHERE id = $1`, created.workspace.ID).Scan(&workspaces); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspace_memberships WHERE workspace_id = $1 AND role = 'owner'`, created.workspace.ID).Scan(&owners); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspace_creations WHERE subject = $1 AND idempotency_key = $2`, "subject-concurrent", "concurrent-key").Scan(&retries); err != nil {
			t.Fatal(err)
		}
		if workspaces != 1 || owners != 1 || retries != 1 {
			t.Fatalf("rows = workspaces %d, owners %d, retries %d; want 1 each", workspaces, owners, retries)
		}
	})

	t.Run("releases an in-progress claim after connection loss", func(t *testing.T) {
		connection, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(connection.Release)

		tx, err := connection.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		lockID := createWorkspaceLockID("subject-connection-loss", "connection-loss-key")
		locked, err := workspacesqlc.New(tx).TryCreateWorkspaceLock(ctx, lockID)
		if err != nil {
			t.Fatal(err)
		}
		if !locked {
			t.Fatal("failed to acquire setup lock")
		}
		if err := connection.Conn().Close(ctx); err != nil {
			t.Fatal(err)
		}

		deadline := time.Now().Add(5 * time.Second)
		for {
			var released bool
			if err := pool.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, lockID).Scan(&released); err != nil {
				t.Fatal(err)
			}
			if released {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("advisory lock was not released after connection loss")
			}
			time.Sleep(10 * time.Millisecond)
		}

		created, err := createWorkspace(ctx, repository, "subject-connection-loss", "connection-loss-key", "Recovered")
		if err != nil {
			t.Fatal(err)
		}
		if created.ID == "" {
			t.Fatal("retry returned no workspace ID")
		}
	})

	t.Run("hides workspaces from non-members", func(t *testing.T) {
		created, err := createWorkspace(ctx, repository, "subject-read", "read-key", "Private")
		if err != nil {
			t.Fatal(err)
		}

		got, err := repository.GetWorkspace(ctx, "subject-read", created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got != created {
			t.Fatalf("workspace = %+v, want %+v", got, created)
		}

		if _, err := repository.GetWorkspace(ctx, "other-subject", created.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
		if _, err := repository.GetWorkspace(ctx, "subject-read", "00000000-0000-0000-0000-000000000000"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
		if _, err := repository.GetWorkspace(ctx, "subject-read", "not-a-uuid"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("error = %v, want ErrNotFound", err)
		}
	})

	t.Run("allows every membership role to read", func(t *testing.T) {
		created, err := createWorkspace(ctx, repository, "subject-owner", "roles-key", "Roles")
		if err != nil {
			t.Fatal(err)
		}

		members := []struct {
			subject string
			role    string
		}{
			{subject: "subject-viewer", role: "viewer"},
			{subject: "subject-member", role: "member"},
			{subject: "subject-admin", role: "admin"},
		}
		for _, member := range members {
			if _, err := pool.Exec(ctx, `
				INSERT INTO workspace_memberships (workspace_id, subject, role)
				VALUES ($1, $2, $3)
			`, created.ID, member.subject, member.role); err != nil {
				t.Fatal(err)
			}
		}

		for _, subject := range []string{"subject-viewer", "subject-member", "subject-admin", "subject-owner"} {
			got, err := repository.GetWorkspace(ctx, subject, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got != created {
				t.Fatalf("workspace for %q = %+v, want %+v", subject, got, created)
			}
		}
	})

	t.Run("preserves canceled and expired contexts", func(t *testing.T) {
		created, err := createWorkspace(ctx, repository, "subject-context", "context-key", "Context")
		if err != nil {
			t.Fatal(err)
		}

		canceledContext, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := repository.GetWorkspace(canceledContext, "subject-context", created.ID); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled error = %v, want context.Canceled", err)
		}

		expiredContext, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		defer cancel()
		if _, err := repository.GetWorkspace(expiredContext, "subject-context", created.ID); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline error = %v, want context.DeadlineExceeded", err)
		}
	})

	t.Run("rolls back partial creates", func(t *testing.T) {
		var before int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspaces`).Scan(&before); err != nil {
			t.Fatal(err)
		}

		_, err := createWorkspace(ctx, repository, "", "rollback-key", "Must Roll Back")
		if err == nil {
			t.Fatal("create succeeded")
		}

		var after int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspaces`).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if after != before {
			t.Fatalf("workspace count = %d, want %d", after, before)
		}
	})

	t.Run("rolls back an injected failure after create", func(t *testing.T) {
		rollbackErr := errors.New("injected rollback")
		var attempted domain.Workspace
		err := repository.WithinTransaction(ctx, func(tx outbound.WorkspaceTransaction) error {
			var createErr error
			attempted, createErr = tx.CreateWorkspace(ctx, "subject-rollback", "rollback-key", "Must Roll Back")
			if createErr != nil {
				return createErr
			}
			return rollbackErr
		})
		if !errors.Is(err, rollbackErr) {
			t.Fatalf("error = %v, want injected rollback", err)
		}

		var workspaces, owners, retries int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspaces WHERE id = $1`, attempted.ID).Scan(&workspaces); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspace_memberships WHERE workspace_id = $1`, attempted.ID).Scan(&owners); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspace_creations WHERE subject = $1 AND idempotency_key = $2`, "subject-rollback", "rollback-key").Scan(&retries); err != nil {
			t.Fatal(err)
		}
		if workspaces != 0 || owners != 0 || retries != 0 {
			t.Fatalf("rows = workspaces %d, owners %d, retries %d; want 0 each", workspaces, owners, retries)
		}
	})

	t.Run("replaces an expired creation record", func(t *testing.T) {
		first, err := createWorkspace(ctx, repository, "subject-expired", "expired-key", "First")
		if err != nil {
			t.Fatal(err)
		}

		var completedAt, expiresAt time.Time
		if err := pool.QueryRow(ctx, `
			SELECT completed_at, expires_at
			FROM workspace_creations
			WHERE subject = $1 AND idempotency_key = $2
		`, "subject-expired", "expired-key").Scan(&completedAt, &expiresAt); err != nil {
			t.Fatal(err)
		}
		if got := expiresAt.Sub(completedAt); got != 24*time.Hour {
			t.Fatalf("retry window = %s, want 24h", got)
		}

		if _, err := pool.Exec(ctx, `
			UPDATE workspace_creations
			SET completed_at = statement_timestamp() - INTERVAL '25 hours',
			    expires_at = statement_timestamp() - INTERVAL '1 hour'
			WHERE subject = $1 AND idempotency_key = $2
		`, "subject-expired", "expired-key"); err != nil {
			t.Fatal(err)
		}
		second, err := createWorkspace(ctx, repository, "subject-expired", "expired-key", "Second")
		if err != nil {
			t.Fatal(err)
		}
		if second.ID == first.ID {
			t.Fatal("expired key replayed the original workspace")
		}
	})

	t.Run("deletes an expired creation record explicitly", func(t *testing.T) {
		first, err := createWorkspace(ctx, repository, "subject-cleanup", "cleanup-key", "Cleanup")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			UPDATE workspace_creations
			SET completed_at = statement_timestamp() - INTERVAL '25 hours',
			    expires_at = statement_timestamp() - INTERVAL '1 hour'
			WHERE subject = $1 AND idempotency_key = $2
		`, "subject-cleanup", "cleanup-key"); err != nil {
			t.Fatal(err)
		}
		deleted, err := workspacesqlc.New(pool).DeleteExpiredWorkspaceCreation(ctx, workspacesqlc.DeleteExpiredWorkspaceCreationParams{
			Subject:        "subject-cleanup",
			IdempotencyKey: "cleanup-key",
		})
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 1 {
			t.Fatalf("deleted records = %d, want 1", deleted)
		}

		second, err := createWorkspace(ctx, repository, "subject-cleanup", "cleanup-key", "Cleanup Again")
		if err != nil {
			t.Fatal(err)
		}
		if second.ID == first.ID {
			t.Fatal("expired key replayed the original workspace")
		}
	})

	t.Run("rolls back only workspace tables", func(t *testing.T) {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()

		if _, err := db.ExecContext(ctx, `CREATE TABLE migration_sentinel (id integer PRIMARY KEY)`); err != nil {
			t.Fatal(err)
		}
		if err := goose.DownToContext(ctx, db, ".", 1); err != nil {
			t.Fatal(err)
		}
		if err := goose.UpToContext(ctx, db, ".", 2); err != nil {
			t.Fatal(err)
		}
		var total, invalid int
		if err := db.QueryRowContext(ctx, `
			SELECT count(*), count(*) FILTER (WHERE expires_at <> completed_at + INTERVAL '24 hours')
			FROM workspace_creations
		`).Scan(&total, &invalid); err != nil {
			t.Fatal(err)
		}
		if total == 0 || invalid != 0 {
			t.Fatalf("backfilled records = %d, invalid expiry windows = %d", total, invalid)
		}
		if err := goose.DownToContext(ctx, db, ".", 0); err != nil {
			t.Fatal(err)
		}

		for _, table := range []string{"workspaces", "workspace_memberships", "workspace_creations"} {
			var exists bool
			if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if exists {
				t.Fatalf("table %q still exists", table)
			}
		}

		var sentinelExists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass('migration_sentinel') IS NOT NULL`).Scan(&sentinelExists); err != nil {
			t.Fatal(err)
		}
		if !sentinelExists {
			t.Fatal("down migration removed an unrelated table")
		}
	})
}

func createWorkspace(ctx context.Context, repository *WorkspaceRepository, subject, idempotencyKey, name string) (domain.Workspace, error) {
	var workspace domain.Workspace
	err := repository.WithinTransaction(ctx, func(tx outbound.WorkspaceTransaction) error {
		var createErr error
		workspace, createErr = tx.CreateWorkspace(ctx, subject, idempotencyKey, name)
		return createErr
	})
	return workspace, err
}

func applyMigrations(t *testing.T, ctx context.Context, dsn string) {
	t.Helper()

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
}
