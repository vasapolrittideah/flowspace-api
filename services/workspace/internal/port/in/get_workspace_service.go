package inbound

import (
	"context"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
)

type GetWorkspaceInput struct {
	Subject     string
	WorkspaceID string
}

type GetWorkspaceService interface {
	GetWorkspace(ctx context.Context, input GetWorkspaceInput) (domain.Workspace, error)
}
