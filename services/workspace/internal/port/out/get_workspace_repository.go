package outbound

import (
	"context"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
)

type GetWorkspaceRepository interface {
	GetWorkspace(ctx context.Context, subject, workspaceID string) (domain.Workspace, error)
}
