package outbound

import (
	"context"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
)

type WorkspaceRepository interface {
	WithinTransaction(ctx context.Context, fn func(WorkspaceTransaction) error) error
	GetWorkspace(ctx context.Context, subject, workspaceID string) (domain.Workspace, error)
}

type WorkspaceTransaction interface {
	CreateWorkspace(ctx context.Context, subject, idempotencyKey, name string) (domain.Workspace, error)
}
