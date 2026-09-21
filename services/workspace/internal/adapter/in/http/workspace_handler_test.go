package httptransport

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
	idempotencyKey := "!" + strings.Repeat("a", 253) + "~"
	var gotInput inbound.CreateWorkspaceInput
	handler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{create: func(_ context.Context, input inbound.CreateWorkspaceInput) (domain.Workspace, error) {
			gotInput = input
			return domain.Workspace{ID: "workspace-1", Name: "Flow Space", CreatedAt: createdAt}, nil
		}},
		fakeTokenVerifier{subject: "user-1"},
		zap.NewNop(),
	)

	response, err := handler.CreateWorkspace(authenticatedContext(idempotencyKey), &workspacev1.CreateWorkspaceRequest{Name: " Flow Space "})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if gotInput != (inbound.CreateWorkspaceInput{Subject: "user-1", IdempotencyKey: idempotencyKey, Name: "Flow Space"}) {
		t.Fatalf("input = %+v", gotInput)
	}
	if got := response.GetWorkspace(); got.GetId() != "workspace-1" || got.GetName() != "Flow Space" || !got.GetCreatedAt().AsTime().Equal(createdAt) {
		t.Fatalf("workspace = %+v", got)
	}
}

func TestWorkspaceHandlerGetWorkspace(t *testing.T) {
	createdAt := time.Date(2026, time.September, 11, 8, 30, 0, 0, time.UTC)
	var gotInput inbound.GetWorkspaceInput
	handler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{get: func(_ context.Context, input inbound.GetWorkspaceInput) (domain.Workspace, error) {
			gotInput = input
			return domain.Workspace{ID: input.WorkspaceID, Name: "Flow Space", CreatedAt: createdAt}, nil
		}},
		fakeTokenVerifier{subject: "user-1"},
		zap.NewNop(),
	)

	response, err := handler.GetWorkspace(authenticatedContext(""), &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if gotInput != (inbound.GetWorkspaceInput{Subject: "user-1", WorkspaceID: "workspace-1"}) {
		t.Fatalf("input = %+v", gotInput)
	}
	if got := response.GetWorkspace(); got.GetId() != "workspace-1" || got.GetName() != "Flow Space" || !got.GetCreatedAt().AsTime().Equal(createdAt) {
		t.Fatalf("workspace = %+v", got)
	}
}

func TestWorkspaceHandlerRejectsInvalidNameBeforeUsecase(t *testing.T) {
	calls := 0
	handler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{create: func(context.Context, inbound.CreateWorkspaceInput) (domain.Workspace, error) {
			calls++
			return domain.Workspace{}, nil
		}},
		fakeTokenVerifier{subject: "user-1"},
		zap.NewNop(),
	)

	_, err := handler.CreateWorkspace(authenticatedContext("request-1"), &workspacev1.CreateWorkspaceRequest{Name: " "})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
	if calls != 0 {
		t.Fatalf("usecase calls = %d, want 0", calls)
	}
	assertFieldViolation(t, err, "name")
}

func TestWorkspaceHandlerRejectsMissingWorkspaceIDBeforeUsecase(t *testing.T) {
	calls := 0
	handler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{get: func(context.Context, inbound.GetWorkspaceInput) (domain.Workspace, error) {
			calls++
			return domain.Workspace{}, nil
		}},
		fakeTokenVerifier{subject: "user-1"},
		zap.NewNop(),
	)

	_, err := handler.GetWorkspace(authenticatedContext(""), &workspacev1.GetWorkspaceRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
	if calls != 0 {
		t.Fatalf("usecase calls = %d, want 0", calls)
	}
	assertFieldViolation(t, err, "workspace_id")
}

func TestWorkspaceHandlerRejectsInvalidBearerToken(t *testing.T) {
	handler := NewWorkspaceHandler(&fakeWorkspaceUsecase{}, fakeTokenVerifier{err: errors.New("invalid token")}, zap.NewNop())

	_, err := handler.GetWorkspace(metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Basic token")), &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestWorkspaceHandlerRequiresOneBearerHeaderForBothMethods(t *testing.T) {
	tests := []struct {
		name          string
		authorization []string
		verifierError error
	}{
		{name: "missing"},
		{name: "blank", authorization: []string{" "}},
		{name: "duplicate", authorization: []string{"Bearer token", "Bearer another"}},
		{name: "wrong scheme", authorization: []string{"Basic token"}},
		{name: "missing token", authorization: []string{"Bearer"}},
		{name: "extra value", authorization: []string{"Bearer token extra"}},
		{name: "invalid token", authorization: []string{"Bearer token"}, verifierError: errors.New("invalid token")},
	}

	for _, tt := range tests {
		for _, method := range []string{"create", "get"} {
			t.Run(tt.name+"/"+method, func(t *testing.T) {
				calls := 0
				handler := NewWorkspaceHandler(
					&fakeWorkspaceUsecase{
						create: func(context.Context, inbound.CreateWorkspaceInput) (domain.Workspace, error) {
							calls++
							return domain.Workspace{}, nil
						},
						get: func(context.Context, inbound.GetWorkspaceInput) (domain.Workspace, error) {
							calls++
							return domain.Workspace{}, nil
						},
					},
					fakeTokenVerifier{subject: "user-1", err: tt.verifierError},
					zap.NewNop(),
				)
				pairs := []string{"idempotency-key", "request-1"}
				for _, authorization := range tt.authorization {
					pairs = append(pairs, "authorization", authorization)
				}
				ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(pairs...))

				var err error
				if method == "create" {
					_, err = handler.CreateWorkspace(ctx, &workspacev1.CreateWorkspaceRequest{Name: "Flow Space"})
				} else {
					_, err = handler.GetWorkspace(ctx, &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
				}
				if status.Code(err) != codes.Unauthenticated {
					t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
				}
				if calls != 0 {
					t.Fatalf("usecase calls = %d, want 0", calls)
				}
			})
		}
	}
}

func TestWorkspaceHandlerRejectsInvalidIdempotencyKeysBeforeUsecase(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{name: "missing"},
		{name: "blank", values: []string{" "}},
		{name: "duplicate", values: []string{"request-1", "request-2"}},
		{name: "too long", values: []string{strings.Repeat("a", 256)}},
		{name: "space", values: []string{"request key"}},
		{name: "tab", values: []string{"request\tkey"}},
		{name: "non ASCII", values: []string{"request-é"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			handler := NewWorkspaceHandler(
				&fakeWorkspaceUsecase{create: func(context.Context, inbound.CreateWorkspaceInput) (domain.Workspace, error) {
					calls++
					return domain.Workspace{}, nil
				}},
				fakeTokenVerifier{subject: "user-1"},
				zap.NewNop(),
			)
			pairs := []string{"authorization", "Bearer token"}
			for _, value := range tt.values {
				pairs = append(pairs, "idempotency-key", value)
			}
			ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(pairs...))

			_, err := handler.CreateWorkspace(ctx, &workspacev1.CreateWorkspaceRequest{Name: "Flow Space"})
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
			}
			if calls != 0 {
				t.Fatalf("usecase calls = %d, want 0", calls)
			}
			assertFieldViolation(t, err, "idempotency_key")
		})
	}
}

func TestWorkspaceHandlerMapsTokenContextErrors(t *testing.T) {
	tests := []struct {
		name   string
		ctx    func() (context.Context, context.CancelFunc)
		code   codes.Code
		wanted error
	}{
		{
			name: "canceled",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx, func() {}
			},
			code:   codes.Canceled,
			wanted: context.Canceled,
		},
		{
			name: "deadline exceeded",
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			},
			code:   codes.DeadlineExceeded,
			wanted: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.ctx()
			defer cancel()
			handler := NewWorkspaceHandler(
				&fakeWorkspaceUsecase{},
				fakeTokenVerifier{verify: func(gotContext context.Context, _ string) (string, error) {
					if !errors.Is(gotContext.Err(), tt.wanted) {
						t.Fatalf("context error = %v, want %v", gotContext.Err(), tt.wanted)
					}
					return "", gotContext.Err()
				}},
				zap.NewNop(),
			)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer token"))

			_, err := handler.GetWorkspace(ctx, &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
			if status.Code(err) != tt.code {
				t.Fatalf("code = %v, want %v", status.Code(err), tt.code)
			}
		})
	}
}

func TestWorkspaceHandlerPropagatesEffectiveDeadline(t *testing.T) {
	tests := []struct {
		name      string
		callerTTL time.Duration
	}{
		{name: "shorter caller deadline", callerTTL: time.Second},
		{name: "five second cap", callerTTL: time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), tt.callerTTL)
			defer cancel()
			callerDeadline, _ := ctx.Deadline()
			var tokenDeadline time.Time
			var usecaseDeadline time.Time
			handler := NewWorkspaceHandler(
				&fakeWorkspaceUsecase{get: func(ctx context.Context, _ inbound.GetWorkspaceInput) (domain.Workspace, error) {
					usecaseDeadline, _ = ctx.Deadline()
					return domain.Workspace{ID: "workspace-1"}, nil
				}},
				fakeTokenVerifier{verify: func(ctx context.Context, _ string) (string, error) {
					tokenDeadline, _ = ctx.Deadline()
					return "user-1", nil
				}},
				zap.NewNop(),
			)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer token"))

			if _, err := handler.GetWorkspace(ctx, &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"}); err != nil {
				t.Fatal(err)
			}
			if tokenDeadline != usecaseDeadline {
				t.Fatalf("token deadline = %v, usecase deadline = %v", tokenDeadline, usecaseDeadline)
			}
			if tt.callerTTL < requestTimeout && tokenDeadline != callerDeadline {
				t.Fatalf("deadline = %v, want caller deadline %v", tokenDeadline, callerDeadline)
			}
			if tt.callerTTL > requestTimeout {
				remaining := time.Until(tokenDeadline)
				if remaining > requestTimeout {
					t.Fatalf("deadline remaining = %v, want at most %v", remaining, requestTimeout)
				}
			}
		})
	}
}

func TestWorkspaceHandlerPropagatesCancellationToUsecase(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	handler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{get: func(gotContext context.Context, _ inbound.GetWorkspaceInput) (domain.Workspace, error) {
			if !errors.Is(gotContext.Err(), context.Canceled) {
				t.Fatalf("context error = %v, want context.Canceled", gotContext.Err())
			}
			return domain.Workspace{}, gotContext.Err()
		}},
		fakeTokenVerifier{subject: "user-1"},
		zap.NewNop(),
	)
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer token"))

	_, err := handler.GetWorkspace(ctx, &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
	if status.Code(err) != codes.Canceled {
		t.Fatalf("code = %v, want Canceled", status.Code(err))
	}
}

func TestWorkspaceHandlerMapsDomainErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		code    codes.Code
		message string
	}{
		{name: "unauthenticated", err: domain.ErrUnauthenticated, code: codes.Unauthenticated, message: "authentication required"},
		{name: "invalid argument", err: &domain.InvalidArgumentError{Field: "workspace_id", Reason: "is invalid"}, code: codes.InvalidArgument, message: "invalid request"},
		{name: "not found", err: domain.ErrNotFound, code: codes.NotFound, message: "workspace not found"},
		{name: "idempotency conflict", err: domain.ErrIdempotencyConflict, code: codes.AlreadyExists, message: "idempotency key conflicts with an earlier request"},
		{name: "create in progress", err: domain.ErrCreateInProgress, code: codes.Aborted, message: "workspace creation is in progress"},
		{name: "deadline", err: context.DeadlineExceeded, code: codes.DeadlineExceeded, message: "request deadline exceeded"},
		{name: "canceled", err: context.Canceled, code: codes.Canceled, message: "request canceled"},
		{name: "unknown", err: errors.New("database unavailable"), code: codes.Internal, message: "internal error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewWorkspaceHandler(
				&fakeWorkspaceUsecase{get: func(context.Context, inbound.GetWorkspaceInput) (domain.Workspace, error) {
					return domain.Workspace{}, tt.err
				}},
				fakeTokenVerifier{subject: "user-1"},
				zap.NewNop(),
			)

			_, err := handler.GetWorkspace(authenticatedContext(""), &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
			if status.Code(err) != tt.code {
				t.Fatalf("code = %v, want %v", status.Code(err), tt.code)
			}
			if status.Convert(err).Message() != tt.message {
				t.Fatalf("message = %q, want %q", status.Convert(err).Message(), tt.message)
			}
			if tt.code == codes.InvalidArgument {
				assertFieldViolation(t, err, "workspace_id")
			}
		})
	}
}

func TestWorkspaceHandlerLogsCompletedCreate(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	handler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{create: func(context.Context, inbound.CreateWorkspaceInput) (domain.Workspace, error) {
			return domain.Workspace{ID: "workspace-1", Name: "Flow Space"}, nil
		}},
		fakeTokenVerifier{subject: "user-1"},
		zap.New(core),
	)

	_, err := handler.CreateWorkspace(requestContext("request-1", "idempotency-1"), &workspacev1.CreateWorkspaceRequest{Name: "Flow Space"})
	if err != nil {
		t.Fatal(err)
	}
	entries := logs.AllUntimed()
	if len(entries) != 1 || entries[0].Message != "request_completed" {
		t.Fatalf("logs = %v", entries)
	}
	completed := entries[0].ContextMap()
	if completed["operation"] != workspacev1.WorkspaceService_CreateWorkspace_FullMethodName || completed["outcome"] != "success" || completed["status"] != "OK" || completed["request_id"] != "request-1" || completed["workspace_id"] != "workspace-1" || completed["duration"] == nil {
		t.Fatalf("request_completed fields = %v", completed)
	}
	logged := fmt.Sprint(completed)
	for _, secret := range []string{"Bearer token", "token", "idempotency-1", "user-1"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("request_completed fields contain %q: %v", secret, completed)
		}
	}
}

func TestWorkspaceHandlerLogsInternalFailure(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	handler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{get: func(context.Context, inbound.GetWorkspaceInput) (domain.Workspace, error) {
			return domain.Workspace{}, errors.New("database unavailable")
		}},
		fakeTokenVerifier{subject: "user-1"},
		zap.New(core),
	)

	_, err := handler.GetWorkspace(requestContext("request-2", ""), &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal", status.Code(err))
	}
	entries := logs.AllUntimed()
	if len(entries) != 1 || entries[0].Message != "request_completed" || entries[0].Level != zap.ErrorLevel {
		t.Fatalf("logs = %v", entries)
	}
	fields := entries[0].ContextMap()
	if fields["operation"] != workspacev1.WorkspaceService_GetWorkspace_FullMethodName || fields["outcome"] != "failure" || fields["status"] != "Internal" || fields["request_id"] != "request-2" || fields["error"] != "database unavailable" {
		t.Fatalf("request_completed fields = %v", fields)
	}
}

func authenticatedContext(idempotencyKey string) context.Context {
	pairs := []string{"authorization", "Bearer token"}
	if idempotencyKey != "" {
		pairs = append(pairs, "idempotency-key", idempotencyKey)
	}
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(pairs...))
}

func requestContext(requestID, idempotencyKey string) context.Context {
	pairs := []string{"authorization", "Bearer token", "x-request-id", requestID}
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
	verify  func(context.Context, string) (string, error)
}

func (f fakeTokenVerifier) VerifyToken(ctx context.Context, token string) (string, error) {
	if f.verify != nil {
		return f.verify(ctx, token)
	}
	return f.subject, f.err
}
