package inbound

import (
	"context"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
)

type CreateWorkspaceInput struct {
	Subject        string
	IdempotencyKey string
	Name           string
}

type CreateWorkspaceService interface {
	CreateWorkspace(ctx context.Context, input CreateWorkspaceInput) (domain.Workspace, error)
}
