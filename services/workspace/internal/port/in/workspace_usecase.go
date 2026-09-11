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

type GetWorkspaceInput struct {
	Subject     string
	WorkspaceID string
}

type WorkspaceUsecase interface {
	CreateWorkspace(ctx context.Context, input CreateWorkspaceInput) (domain.Workspace, error)
	GetWorkspace(ctx context.Context, input GetWorkspaceInput) (domain.Workspace, error)
}
