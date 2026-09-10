package outbound

import (
	"context"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
)

type WorkspaceRepository interface {
	CreateWorkspace(ctx context.Context, subject, idempotencyKey, name string) (domain.Workspace, error)
	GetWorkspace(ctx context.Context, subject, workspaceID string) (domain.Workspace, error)
}
