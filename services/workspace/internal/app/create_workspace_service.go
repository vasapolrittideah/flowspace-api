package app

import (
	"context"
	"fmt"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

const maxIdempotencyKeyBytes = 255

type CreateWorkspaceService struct {
	repository outbound.CreateWorkspaceRepository
}

var _ inbound.CreateWorkspaceService = (*CreateWorkspaceService)(nil)

func NewCreateWorkspaceService(repository outbound.CreateWorkspaceRepository) *CreateWorkspaceService {
	return &CreateWorkspaceService{repository: repository}
}

func (s *CreateWorkspaceService) CreateWorkspace(ctx context.Context, input inbound.CreateWorkspaceInput) (domain.Workspace, error) {
	if input.Subject == "" {
		return domain.Workspace{}, domain.ErrUnauthenticated
	}
	if input.IdempotencyKey == "" {
		return domain.Workspace{}, invalid("idempotency_key", "is required")
	}
	if len(input.IdempotencyKey) > maxIdempotencyKeyBytes {
		return domain.Workspace{}, invalid("idempotency_key", "must be at most 255 bytes")
	}
	for index := range len(input.IdempotencyKey) {
		if input.IdempotencyKey[index] < 0x21 || input.IdempotencyKey[index] > 0x7e {
			return domain.Workspace{}, invalid("idempotency_key", "must contain only visible ASCII characters")
		}
	}

	newWorkspace, err := domain.NewWorkspace(input.Name)
	if err != nil {
		return domain.Workspace{}, err
	}

	var workspace domain.Workspace
	err = s.repository.WithinTransaction(ctx, func(tx outbound.WorkspaceTransaction) error {
		var createErr error
		workspace, createErr = tx.CreateWorkspace(ctx, input.Subject, input.IdempotencyKey, newWorkspace.Name)
		return createErr
	})
	if err != nil {
		return domain.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	return workspace, nil
}

func invalid(field, reason string) error {
	return &domain.InvalidArgumentError{Field: field, Reason: reason}
}
