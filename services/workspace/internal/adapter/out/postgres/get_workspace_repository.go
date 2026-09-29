package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	workspacesqlc "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
)

func (r *WorkspaceRepository) GetWorkspace(ctx context.Context, subject, workspaceID string) (domain.Workspace, error) {
	id, err := parseUUID(workspaceID)
	if err != nil {
		return domain.Workspace{}, domain.ErrNotFound
	}
	row, err := workspacesqlc.New(r.pool).GetWorkspaceForSubject(ctx, workspacesqlc.GetWorkspaceForSubjectParams{
		WorkspaceID: id,
		Subject:     subject,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Workspace{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	return domain.Workspace{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt.Time}, nil
}
