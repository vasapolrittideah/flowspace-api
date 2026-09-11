package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
)

func TestWorkspaceServiceCreateWorkspace(t *testing.T) {
	want := domain.Workspace{ID: "workspace-id", Name: "Platform", CreatedAt: time.Now()}
	repository := &fakeWorkspaceRepository{
		create: func(_ context.Context, subject, idempotencyKey, name string) (domain.Workspace, error) {
			if subject != "subject-1" || idempotencyKey != "request-1" || name != "Platform" {
				t.Fatalf("unexpected create input: %q, %q, %q", subject, idempotencyKey, name)
			}
			return want, nil
		},
	}

	got, err := NewWorkspaceService(repository).CreateWorkspace(context.Background(), inbound.CreateWorkspaceInput{
		Subject:        "subject-1",
		IdempotencyKey: "request-1",
		Name:           "  Platform  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("workspace = %+v, want %+v", got, want)
	}
}

func TestWorkspaceServiceRejectsInvalidCreate(t *testing.T) {
	tests := []struct {
		name  string
		input inbound.CreateWorkspaceInput
		want  error
	}{
		{name: "missing subject", input: inbound.CreateWorkspaceInput{IdempotencyKey: "key", Name: "Workspace"}, want: domain.ErrUnauthenticated},
		{name: "missing idempotency key", input: inbound.CreateWorkspaceInput{Subject: "subject", Name: "Workspace"}, want: domain.ErrInvalidArgument},
		{name: "long idempotency key", input: inbound.CreateWorkspaceInput{Subject: "subject", IdempotencyKey: strings.Repeat("k", 256), Name: "Workspace"}, want: domain.ErrInvalidArgument},
		{name: "blank name", input: inbound.CreateWorkspaceInput{Subject: "subject", IdempotencyKey: "key", Name: " \t"}, want: domain.ErrInvalidArgument},
		{name: "long name", input: inbound.CreateWorkspaceInput{Subject: "subject", IdempotencyKey: "key", Name: strings.Repeat("界", 101)}, want: domain.ErrInvalidArgument},
		{name: "invalid name encoding", input: inbound.CreateWorkspaceInput{Subject: "subject", IdempotencyKey: "key", Name: string([]byte{0xff})}, want: domain.ErrInvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeWorkspaceRepository{
				create: func(context.Context, string, string, string) (domain.Workspace, error) {
					t.Fatal("repository called for invalid input")
					return domain.Workspace{}, nil
				},
			}
			_, err := NewWorkspaceService(repository).CreateWorkspace(context.Background(), tt.input)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestWorkspaceServiceGetWorkspace(t *testing.T) {
	want := domain.Workspace{ID: "workspace-id", Name: "Platform", CreatedAt: time.Now()}
	repository := &fakeWorkspaceRepository{
		get: func(_ context.Context, subject, workspaceID string) (domain.Workspace, error) {
			if subject != "subject-1" || workspaceID != "workspace-id" {
				t.Fatalf("unexpected get input: %q, %q", subject, workspaceID)
			}
			return want, nil
		},
	}

	got, err := NewWorkspaceService(repository).GetWorkspace(context.Background(), inbound.GetWorkspaceInput{
		Subject:     "subject-1",
		WorkspaceID: "workspace-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("workspace = %+v, want %+v", got, want)
	}
}

func TestWorkspaceServiceRejectsInvalidGet(t *testing.T) {
	tests := []struct {
		name  string
		input inbound.GetWorkspaceInput
		want  error
	}{
		{name: "missing subject", input: inbound.GetWorkspaceInput{WorkspaceID: "workspace-id"}, want: domain.ErrUnauthenticated},
		{name: "missing workspace id", input: inbound.GetWorkspaceInput{Subject: "subject"}, want: domain.ErrInvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeWorkspaceRepository{
				get: func(context.Context, string, string) (domain.Workspace, error) {
					t.Fatal("repository called for invalid input")
					return domain.Workspace{}, nil
				},
			}
			_, err := NewWorkspaceService(repository).GetWorkspace(context.Background(), tt.input)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestWorkspaceServicePreservesRepositoryErrors(t *testing.T) {
	repository := &fakeWorkspaceRepository{
		create: func(context.Context, string, string, string) (domain.Workspace, error) {
			return domain.Workspace{}, domain.ErrIdempotencyConflict
		},
		get: func(context.Context, string, string) (domain.Workspace, error) {
			return domain.Workspace{}, domain.ErrNotFound
		},
	}
	service := NewWorkspaceService(repository)

	_, err := service.CreateWorkspace(context.Background(), inbound.CreateWorkspaceInput{Subject: "subject", IdempotencyKey: "key", Name: "Workspace"})
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("create error = %v, want ErrIdempotencyConflict", err)
	}
	_, err = service.GetWorkspace(context.Background(), inbound.GetWorkspaceInput{Subject: "subject", WorkspaceID: "workspace-id"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get error = %v, want ErrNotFound", err)
	}
}

type fakeWorkspaceRepository struct {
	create func(context.Context, string, string, string) (domain.Workspace, error)
	get    func(context.Context, string, string) (domain.Workspace, error)
}

func (r *fakeWorkspaceRepository) CreateWorkspace(ctx context.Context, subject, idempotencyKey, name string) (domain.Workspace, error) {
	return r.create(ctx, subject, idempotencyKey, name)
}

func (r *fakeWorkspaceRepository) GetWorkspace(ctx context.Context, subject, workspaceID string) (domain.Workspace, error) {
	return r.get(ctx, subject, workspaceID)
}
