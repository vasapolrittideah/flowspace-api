package httptransport

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
)

func TestWorkspaceHandlerCreateWorkspace(t *testing.T) {
	createdAt := time.Date(2026, time.September, 11, 8, 30, 0, 0, time.UTC)
	var gotInput inbound.CreateWorkspaceInput
	handler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{create: func(_ context.Context, input inbound.CreateWorkspaceInput) (domain.Workspace, error) {
			gotInput = input
			return domain.Workspace{ID: "workspace-1", Name: "Flow Space", CreatedAt: createdAt}, nil
		}},
		fakeTokenVerifier{subject: "user-1"},
	)

	response, err := handler.CreateWorkspace(authenticatedContext("request-1"), &workspacev1.CreateWorkspaceRequest{Name: " Flow Space "})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if gotInput != (inbound.CreateWorkspaceInput{Subject: "user-1", IdempotencyKey: "request-1", Name: "Flow Space"}) {
		t.Fatalf("input = %+v", gotInput)
	}
	if got := response.GetWorkspace(); got.GetId() != "workspace-1" || got.GetName() != "Flow Space" || !got.GetCreatedAt().AsTime().Equal(createdAt) {
		t.Fatalf("workspace = %+v", got)
	}
}

func TestWorkspaceHandlerGetWorkspace(t *testing.T) {
	var gotInput inbound.GetWorkspaceInput
	handler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{get: func(_ context.Context, input inbound.GetWorkspaceInput) (domain.Workspace, error) {
			gotInput = input
			return domain.Workspace{ID: input.WorkspaceID, Name: "Flow Space"}, nil
		}},
		fakeTokenVerifier{subject: "user-1"},
	)

	_, err := handler.GetWorkspace(authenticatedContext(""), &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if gotInput != (inbound.GetWorkspaceInput{Subject: "user-1", WorkspaceID: "workspace-1"}) {
		t.Fatalf("input = %+v", gotInput)
	}
}

func TestWorkspaceHandlerRejectsInvalidRequestBeforeUsecase(t *testing.T) {
	calls := 0
	handler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{create: func(context.Context, inbound.CreateWorkspaceInput) (domain.Workspace, error) {
			calls++
			return domain.Workspace{}, nil
		}},
		fakeTokenVerifier{subject: "user-1"},
	)

	_, err := handler.CreateWorkspace(authenticatedContext(""), &workspacev1.CreateWorkspaceRequest{Name: "Workspace"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
	if calls != 0 {
		t.Fatalf("usecase calls = %d, want 0", calls)
	}
	assertFieldViolation(t, err, "idempotency_key")
}

func TestWorkspaceHandlerRejectsInvalidBearerToken(t *testing.T) {
	handler := NewWorkspaceHandler(&fakeWorkspaceUsecase{}, fakeTokenVerifier{err: errors.New("invalid token")})

	_, err := handler.GetWorkspace(metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Basic token")), &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestWorkspaceHandlerMapsDomainErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "not found", err: domain.ErrNotFound, code: codes.NotFound},
		{name: "idempotency conflict", err: domain.ErrIdempotencyConflict, code: codes.AlreadyExists},
		{name: "create in progress", err: domain.ErrCreateInProgress, code: codes.Aborted},
		{name: "deadline", err: context.DeadlineExceeded, code: codes.DeadlineExceeded},
		{name: "unknown", err: errors.New("database unavailable"), code: codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewWorkspaceHandler(
				&fakeWorkspaceUsecase{get: func(context.Context, inbound.GetWorkspaceInput) (domain.Workspace, error) {
					return domain.Workspace{}, tt.err
				}},
				fakeTokenVerifier{subject: "user-1"},
			)

			_, err := handler.GetWorkspace(authenticatedContext(""), &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
			if status.Code(err) != tt.code {
				t.Fatalf("code = %v, want %v", status.Code(err), tt.code)
			}
		})
	}
}

func authenticatedContext(idempotencyKey string) context.Context {
	pairs := []string{"authorization", "Bearer token"}
	if idempotencyKey != "" {
		pairs = append(pairs, "idempotency-key", idempotencyKey)
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(pairs...))
}

func assertFieldViolation(t *testing.T, err error, field string) {
	t.Helper()
	for _, detail := range status.Convert(err).Details() {
		if badRequest, ok := detail.(*errdetails.BadRequest); ok && len(badRequest.GetFieldViolations()) == 1 && badRequest.GetFieldViolations()[0].GetField() == field {
			return
		}
	}
	t.Fatalf("error details = %v, want violation for %q", status.Convert(err).Details(), field)
}

type fakeWorkspaceUsecase struct {
	create func(context.Context, inbound.CreateWorkspaceInput) (domain.Workspace, error)
	get    func(context.Context, inbound.GetWorkspaceInput) (domain.Workspace, error)
}

func (f *fakeWorkspaceUsecase) CreateWorkspace(ctx context.Context, input inbound.CreateWorkspaceInput) (domain.Workspace, error) {
	if f.create == nil {
		return domain.Workspace{}, errors.New("unexpected CreateWorkspace call")
	}
	return f.create(ctx, input)
}

func (f *fakeWorkspaceUsecase) GetWorkspace(ctx context.Context, input inbound.GetWorkspaceInput) (domain.Workspace, error) {
	if f.get == nil {
		return domain.Workspace{}, errors.New("unexpected GetWorkspace call")
	}
	return f.get(ctx, input)
}

type fakeTokenVerifier struct {
	subject string
	err     error
}

func (f fakeTokenVerifier) VerifyToken(context.Context, string) (string, error) {
	return f.subject, f.err
}
