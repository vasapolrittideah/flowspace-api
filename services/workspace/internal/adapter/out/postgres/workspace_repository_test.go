//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/db/migrations"
	workspacesqlc "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
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
		created, err := repository.CreateWorkspace(ctx, "subject-create", "create-key", "Platform")
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

		_, err = pool.Exec(ctx, `
			INSERT INTO workspace_memberships (workspace_id, subject, role)
			VALUES ($1, $2, 'owner')
		`, created.ID, "second-owner")
		if err == nil {
			t.Fatal("second owner insert succeeded")
		}
	})

	t.Run("replays the same idempotent create", func(t *testing.T) {
		first, err := repository.CreateWorkspace(ctx, "subject-replay", "replay-key", "Replay")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE workspaces SET name = 'Renamed Later' WHERE id = $1`, first.ID); err != nil {
			t.Fatal(err)
		}
		second, err := repository.CreateWorkspace(ctx, "subject-replay", "replay-key", "Replay")
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

		other, err := repository.CreateWorkspace(ctx, "another-subject", "replay-key", "Replay")
		if err != nil {
			t.Fatal(err)
		}
		if other.ID == first.ID {
			t.Fatal("idempotency key was not scoped to the subject")
		}
	})

	t.Run("rejects an idempotency key reused with another request", func(t *testing.T) {
		if _, err := repository.CreateWorkspace(ctx, "subject-conflict", "conflict-key", "Original"); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.CreateWorkspace(ctx, "subject-conflict", "conflict-key", "Changed"); !errors.Is(err, domain.ErrIdempotencyConflict) {
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

		if _, err := repository.CreateWorkspace(ctx, "subject-in-flight", "in-flight-key", "In Flight"); !errors.Is(err, domain.ErrCreateInProgress) {
			t.Fatalf("error = %v, want ErrCreateInProgress", err)
		}
	})

	t.Run("hides workspaces from non-members", func(t *testing.T) {
		created, err := repository.CreateWorkspace(ctx, "subject-read", "read-key", "Private")
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
	})

	t.Run("rolls back partial creates", func(t *testing.T) {
		var before int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM workspaces`).Scan(&before); err != nil {
			t.Fatal(err)
		}

		_, err := repository.CreateWorkspace(ctx, "", "rollback-key", "Must Roll Back")
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
