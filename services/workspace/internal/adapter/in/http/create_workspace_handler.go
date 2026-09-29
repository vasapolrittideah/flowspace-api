package httptransport

import (
	"context"
	"time"

	"go.uber.org/zap"

	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
)

const maxIdempotencyKeyBytes = 255

func (h *WorkspaceHandler) CreateWorkspace(ctx context.Context, request *workspacev1.CreateWorkspaceRequest) (_ *workspacev1.CreateWorkspaceResponse, err error) {
	logger := h.requestLogger(ctx, workspacev1.WorkspaceService_CreateWorkspace_FullMethodName)
	started := time.Now()
	var cause error
	defer func() { logRequest(logger, err, cause, time.Since(started)) }()

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	subject, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	idempotencyKey, err := requiredMetadata(ctx, "idempotency-key")
	if err != nil {
		return nil, invalidArgument("idempotency_key", err.Error())
	}
	if len(idempotencyKey) > maxIdempotencyKeyBytes {
		return nil, invalidArgument("idempotency_key", "must be at most 255 bytes")
	}
	for index := range len(idempotencyKey) {
		if idempotencyKey[index] < 0x21 || idempotencyKey[index] > 0x7e {
			return nil, invalidArgument("idempotency_key", "must contain only visible ASCII characters")
		}
	}
	if request == nil {
		return nil, invalidArgument("request", "is required")
	}
	newWorkspace, err := domain.NewWorkspace(request.GetName())
	if err != nil {
		return nil, rpcError(err)
	}

	workspace, cause := h.create.CreateWorkspace(ctx, inbound.CreateWorkspaceInput{
		Subject:        subject,
		IdempotencyKey: idempotencyKey,
		Name:           newWorkspace.Name,
	})
	if cause != nil {
		return nil, rpcError(cause)
	}
	logger = logger.With(zap.String("workspace_id", workspace.ID))
	return &workspacev1.CreateWorkspaceResponse{Workspace: workspaceMessage(workspace)}, nil
}
