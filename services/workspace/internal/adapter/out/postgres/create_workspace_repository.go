package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	workspacesqlc "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
)

func (tx *workspaceTransaction) CreateWorkspace(ctx context.Context, subject, idempotencyKey, name string) (domain.Workspace, error) {
	locked, err := tx.queries.TryCreateWorkspaceLock(ctx, createWorkspaceLockID(subject, idempotencyKey))
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("lock workspace create: %w", err)
	}
	if !locked {
		return domain.Workspace{}, domain.ErrCreateInProgress
	}
	if _, err := tx.queries.DeleteExpiredWorkspaceCreation(ctx, workspacesqlc.DeleteExpiredWorkspaceCreationParams{
		Subject:        subject,
		IdempotencyKey: idempotencyKey,
	}); err != nil {
		return domain.Workspace{}, fmt.Errorf("delete expired workspace creation: %w", err)
	}

	requestHash := sha256.Sum256([]byte(name))
	creation, err := tx.queries.GetWorkspaceCreation(ctx, workspacesqlc.GetWorkspaceCreationParams{
		Subject:        subject,
		IdempotencyKey: idempotencyKey,
	})
	if err == nil {
		if !bytes.Equal(creation.RequestHash, requestHash[:]) {
			return domain.Workspace{}, domain.ErrIdempotencyConflict
		}
		return domain.Workspace{ID: creation.ID, Name: creation.Name, CreatedAt: creation.CreatedAt.Time}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Workspace{}, fmt.Errorf("get workspace creation: %w", err)
	}

	created, err := tx.queries.CreateWorkspace(ctx, name)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	workspaceID, err := parseUUID(created.ID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("parse created workspace id: %w", err)
	}
	if err := tx.queries.CreateWorkspaceOwner(ctx, workspacesqlc.CreateWorkspaceOwnerParams{
		WorkspaceID: workspaceID,
		Subject:     subject,
	}); err != nil {
		return domain.Workspace{}, fmt.Errorf("create workspace owner: %w", err)
	}
	if err := tx.queries.RecordWorkspaceCreation(ctx, workspacesqlc.RecordWorkspaceCreationParams{
		Subject:            subject,
		IdempotencyKey:     idempotencyKey,
		RequestHash:        requestHash[:],
		WorkspaceID:        workspaceID,
		WorkspaceName:      created.Name,
		WorkspaceCreatedAt: created.CreatedAt,
	}); err != nil {
		return domain.Workspace{}, fmt.Errorf("record workspace creation: %w", err)
	}
	return domain.Workspace{ID: created.ID, Name: created.Name, CreatedAt: created.CreatedAt.Time}, nil
}
