package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
)

func TestWorkspaceServiceGetWorkspace(t *testing.T) {
	want := domain.Workspace{ID: "workspace-id", Name: "Platform", CreatedAt: time.Now()}
	ctx := t.Context()
	repository := &fakeWorkspaceRepository{
		get: func(gotContext context.Context, subject, workspaceID string) (domain.Workspace, error) {
			if gotContext != ctx {
				t.Fatal("get context differs from request context")
			}
			if subject != "subject-1" || workspaceID != "workspace-id" {
				t.Fatalf("unexpected get input: %q, %q", subject, workspaceID)
			}
			return want, nil
		},
	}

	got, err := NewGetWorkspaceService(repository).GetWorkspace(ctx, inbound.GetWorkspaceInput{
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
			_, err := NewGetWorkspaceService(repository).GetWorkspace(context.Background(), tt.input)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestWorkspaceServicePreservesGetRepositoryError(t *testing.T) {
	repository := &fakeWorkspaceRepository{
		get: func(context.Context, string, string) (domain.Workspace, error) {
			return domain.Workspace{}, domain.ErrNotFound
		},
	}
	_, err := NewGetWorkspaceService(repository).GetWorkspace(context.Background(), inbound.GetWorkspaceInput{Subject: "subject", WorkspaceID: "workspace-id"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get error = %v, want ErrNotFound", err)
	}
}

func TestWorkspaceServicePreservesGetContextErrors(t *testing.T) {
	tests := []struct {
		name string
		want error
	}{
		{name: "canceled", want: context.Canceled},
		{name: "deadline exceeded", want: context.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeWorkspaceRepository{
				get: func(context.Context, string, string) (domain.Workspace, error) {
					return domain.Workspace{}, tt.want
				},
			}

			_, err := NewGetWorkspaceService(repository).GetWorkspace(context.Background(), inbound.GetWorkspaceInput{
				Subject:     "subject",
				WorkspaceID: "workspace-id",
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}
