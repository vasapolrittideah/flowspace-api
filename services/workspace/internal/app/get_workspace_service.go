package app

import (
	"context"
	"fmt"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

type GetWorkspaceService struct {
	repository outbound.GetWorkspaceRepository
}

var _ inbound.GetWorkspaceService = (*GetWorkspaceService)(nil)

func NewGetWorkspaceService(repository outbound.GetWorkspaceRepository) *GetWorkspaceService {
	return &GetWorkspaceService{repository: repository}
}

func (s *GetWorkspaceService) GetWorkspace(ctx context.Context, input inbound.GetWorkspaceInput) (domain.Workspace, error) {
	if input.Subject == "" {
		return domain.Workspace{}, domain.ErrUnauthenticated
	}
	if input.WorkspaceID == "" {
		return domain.Workspace{}, invalid("workspace_id", "is required")
	}

	workspace, err := s.repository.GetWorkspace(ctx, input.Subject, input.WorkspaceID)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	return workspace, nil
}
