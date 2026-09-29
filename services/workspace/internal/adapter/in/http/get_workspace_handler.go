package httptransport

import (
	"context"
	"time"

	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
)

func (h *WorkspaceHandler) GetWorkspace(ctx context.Context, request *workspacev1.GetWorkspaceRequest) (_ *workspacev1.GetWorkspaceResponse, err error) {
	logger := h.requestLogger(ctx, workspacev1.WorkspaceService_GetWorkspace_FullMethodName)
	started := time.Now()
	var cause error
	defer func() { logRequest(logger, err, cause, time.Since(started)) }()

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	subject, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if request == nil || request.GetWorkspaceId() == "" {
		return nil, invalidArgument("workspace_id", "is required")
	}

	workspace, cause := h.get.GetWorkspace(ctx, inbound.GetWorkspaceInput{Subject: subject, WorkspaceID: request.GetWorkspaceId()})
	if cause != nil {
		return nil, rpcError(cause)
	}
	return &workspacev1.GetWorkspaceResponse{Workspace: workspaceMessage(workspace)}, nil
}
