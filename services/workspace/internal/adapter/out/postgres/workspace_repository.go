package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	workspacesqlc "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
)

var (
	ErrCreateInProgress    = errors.New("workspace create in progress")
	ErrIdempotencyConflict = errors.New("idempotency key reused with another request")
	ErrNotFound            = errors.New("workspace not found")
)

type WorkspaceRepository struct {
	pool *pgxpool.Pool
}

type CreateWorkspaceParams struct {
	Subject        string
	IdempotencyKey string
	Name           string
}

func New(pool *pgxpool.Pool) *WorkspaceRepository {
	return &WorkspaceRepository{pool: pool}
}

func (r *WorkspaceRepository) CreateWorkspace(ctx context.Context, params CreateWorkspaceParams) (domain.Workspace, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("begin create workspace: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := workspacesqlc.New(tx)
	locked, err := queries.TryCreateWorkspaceLock(ctx, createWorkspaceLockID(params.Subject, params.IdempotencyKey))
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("lock workspace create: %w", err)
	}
	if !locked {
		return domain.Workspace{}, ErrCreateInProgress
	}

	requestHash := sha256.Sum256([]byte(params.Name))
	creation, err := queries.GetWorkspaceCreation(ctx, workspacesqlc.GetWorkspaceCreationParams{
		Subject:        params.Subject,
		IdempotencyKey: params.IdempotencyKey,
	})
	if err == nil {
		if !bytes.Equal(creation.RequestHash, requestHash[:]) {
			return domain.Workspace{}, ErrIdempotencyConflict
		}
		workspace := domain.Workspace{ID: creation.ID, Name: creation.Name, CreatedAt: creation.CreatedAt.Time}
		if err := tx.Commit(ctx); err != nil {
			return domain.Workspace{}, fmt.Errorf("commit workspace replay: %w", err)
		}
		return workspace, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Workspace{}, fmt.Errorf("get workspace creation: %w", err)
	}

	created, err := queries.CreateWorkspace(ctx, params.Name)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	workspaceID, err := parseUUID(created.ID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("parse created workspace id: %w", err)
	}
	if err := queries.CreateWorkspaceOwner(ctx, workspacesqlc.CreateWorkspaceOwnerParams{
		WorkspaceID: workspaceID,
		Subject:     params.Subject,
	}); err != nil {
		return domain.Workspace{}, fmt.Errorf("create workspace owner: %w", err)
	}
	if err := queries.RecordWorkspaceCreation(ctx, workspacesqlc.RecordWorkspaceCreationParams{
		Subject:            params.Subject,
		IdempotencyKey:     params.IdempotencyKey,
		RequestHash:        requestHash[:],
		WorkspaceID:        workspaceID,
		WorkspaceName:      created.Name,
		WorkspaceCreatedAt: created.CreatedAt,
	}); err != nil {
		return domain.Workspace{}, fmt.Errorf("record workspace creation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Workspace{}, fmt.Errorf("commit workspace create: %w", err)
	}

	return domain.Workspace{ID: created.ID, Name: created.Name, CreatedAt: created.CreatedAt.Time}, nil
}

func (r *WorkspaceRepository) GetWorkspace(ctx context.Context, subject, workspaceID string) (domain.Workspace, error) {
	id, err := parseUUID(workspaceID)
	if err != nil {
		return domain.Workspace{}, ErrNotFound
	}
	row, err := workspacesqlc.New(r.pool).GetWorkspaceForSubject(ctx, workspacesqlc.GetWorkspaceForSubjectParams{
		WorkspaceID: id,
		Subject:     subject,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Workspace{}, ErrNotFound
	}
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	return domain.Workspace{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt.Time}, nil
}

func createWorkspaceLockID(subject, idempotencyKey string) int64 {
	hash := sha256.Sum256([]byte("flowspace.workspace.v1.WorkspaceService/CreateWorkspace\x00" + subject + "\x00" + idempotencyKey))
	return int64(binary.BigEndian.Uint64(hash[:8]))
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(value)
	return id, err
}
