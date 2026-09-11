package httptransport

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

const (
	requestTimeout         = 5 * time.Second
	maxIdempotencyKeyBytes = 255
)

type WorkspaceHandler struct {
	workspacev1.UnimplementedWorkspaceServiceServer
	usecase  inbound.WorkspaceUsecase
	verifier outbound.TokenVerifier
}

var _ workspacev1.WorkspaceServiceServer = (*WorkspaceHandler)(nil)

func NewWorkspaceHandler(usecase inbound.WorkspaceUsecase, verifier outbound.TokenVerifier) *WorkspaceHandler {
	return &WorkspaceHandler{usecase: usecase, verifier: verifier}
}

func (h *WorkspaceHandler) CreateWorkspace(ctx context.Context, request *workspacev1.CreateWorkspaceRequest) (*workspacev1.CreateWorkspaceResponse, error) {
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
	if request == nil {
		return nil, invalidArgument("request", "is required")
	}
	newWorkspace, err := domain.NewWorkspace(request.GetName())
	if err != nil {
		return nil, rpcError(err)
	}

	workspace, err := h.usecase.CreateWorkspace(ctx, inbound.CreateWorkspaceInput{
		Subject:        subject,
		IdempotencyKey: idempotencyKey,
		Name:           newWorkspace.Name,
	})
	if err != nil {
		return nil, rpcError(err)
	}
	return &workspacev1.CreateWorkspaceResponse{Workspace: workspaceMessage(workspace)}, nil
}

func (h *WorkspaceHandler) GetWorkspace(ctx context.Context, request *workspacev1.GetWorkspaceRequest) (*workspacev1.GetWorkspaceResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	subject, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if request == nil || request.GetWorkspaceId() == "" {
		return nil, invalidArgument("workspace_id", "is required")
	}

	workspace, err := h.usecase.GetWorkspace(ctx, inbound.GetWorkspaceInput{Subject: subject, WorkspaceID: request.GetWorkspaceId()})
	if err != nil {
		return nil, rpcError(err)
	}
	return &workspacev1.GetWorkspaceResponse{Workspace: workspaceMessage(workspace)}, nil
}

func (h *WorkspaceHandler) authenticate(ctx context.Context) (string, error) {
	authorization, err := requiredMetadata(ctx, "authorization")
	if err != nil {
		return "", status.Error(codes.Unauthenticated, "authentication required")
	}
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", status.Error(codes.Unauthenticated, "authentication required")
	}
	subject, err := h.verifier.VerifyToken(ctx, parts[1])
	if err != nil || subject == "" {
		return "", status.Error(codes.Unauthenticated, "authentication required")
	}
	return subject, nil
}

func requiredMetadata(ctx context.Context, key string) (string, error) {
	values := metadata.ValueFromIncomingContext(ctx, key)
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return "", errors.New("must be provided once")
	}
	return values[0], nil
}

func rpcError(err error) error {
	switch {
	case errors.Is(err, domain.ErrUnauthenticated):
		return status.Error(codes.Unauthenticated, "authentication required")
	case errors.Is(err, domain.ErrInvalidArgument):
		if invalid, ok := errors.AsType[*domain.InvalidArgumentError](err); ok {
			return invalidArgument(invalid.Field, invalid.Reason)
		}
		return status.Error(codes.InvalidArgument, "invalid request")
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "workspace not found")
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return status.Error(codes.AlreadyExists, "idempotency key conflicts with an earlier request")
	case errors.Is(err, domain.ErrCreateInProgress):
		return status.Error(codes.Aborted, "workspace creation is in progress")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func invalidArgument(field, reason string) error {
	result, err := status.New(codes.InvalidArgument, "invalid request").WithDetails(&errdetails.BadRequest{
		FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: field, Description: reason}},
	})
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid request")
	}
	return result.Err()
}

func workspaceMessage(workspace domain.Workspace) *workspacev1.Workspace {
	return &workspacev1.Workspace{
		Id:        workspace.ID,
		Name:      workspace.Name,
		CreatedAt: timestamppb.New(workspace.CreatedAt),
	}
}
