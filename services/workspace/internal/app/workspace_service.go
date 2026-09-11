package app

import (
	"context"
	"fmt"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

const maxIdempotencyKeyBytes = 255

type WorkspaceService struct {
	repository outbound.WorkspaceRepository
}

var _ inbound.WorkspaceUsecase = (*WorkspaceService)(nil)

func NewWorkspaceService(repository outbound.WorkspaceRepository) *WorkspaceService {
	return &WorkspaceService{repository: repository}
}

func (s *WorkspaceService) CreateWorkspace(ctx context.Context, input inbound.CreateWorkspaceInput) (domain.Workspace, error) {
	if input.Subject == "" {
		return domain.Workspace{}, domain.ErrUnauthenticated
	}
	if input.IdempotencyKey == "" {
		return domain.Workspace{}, invalid("idempotency_key", "is required")
	}
	if len(input.IdempotencyKey) > maxIdempotencyKeyBytes {
		return domain.Workspace{}, invalid("idempotency_key", "must be at most 255 bytes")
	}

	newWorkspace, err := domain.NewWorkspace(input.Name)
	if err != nil {
		return domain.Workspace{}, err
	}

	workspace, err := s.repository.CreateWorkspace(ctx, input.Subject, input.IdempotencyKey, newWorkspace.Name)
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	return workspace, nil
}

func (s *WorkspaceService) GetWorkspace(ctx context.Context, input inbound.GetWorkspaceInput) (domain.Workspace, error) {
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

func invalid(field, reason string) error {
	return &domain.InvalidArgumentError{Field: field, Reason: reason}
}
