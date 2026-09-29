package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	workspacesqlc "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

type WorkspaceRepository struct {
	pool *pgxpool.Pool
}

var (
	_ outbound.CreateWorkspaceRepository = (*WorkspaceRepository)(nil)
	_ outbound.GetWorkspaceRepository    = (*WorkspaceRepository)(nil)
)

func NewWorkspaceRepository(pool *pgxpool.Pool) *WorkspaceRepository {
	return &WorkspaceRepository{pool: pool}
}

func (r *WorkspaceRepository) WithinTransaction(ctx context.Context, fn func(outbound.WorkspaceTransaction) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin workspace transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(&workspaceTransaction{queries: workspacesqlc.New(tx)}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit workspace transaction: %w", err)
	}
	return nil
}

type workspaceTransaction struct {
	queries *workspacesqlc.Queries
}

var _ outbound.WorkspaceTransaction = (*workspaceTransaction)(nil)

func createWorkspaceLockID(subject, idempotencyKey string) int64 {
	hash := sha256.Sum256([]byte("flowspace.workspace.v1.WorkspaceService/CreateWorkspace\x00" + subject + "\x00" + idempotencyKey))
	return int64(binary.BigEndian.Uint64(hash[:8])) //nolint:gosec // PostgreSQL advisory locks accept signed bit patterns.
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(value)
	return id, err
}
