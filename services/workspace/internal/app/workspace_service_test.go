package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

func TestWorkspaceServiceCreateWorkspace(t *testing.T) {
	want := domain.Workspace{ID: "workspace-id", Name: "Platform", CreatedAt: time.Now()}
	ctx := t.Context()
	repository := &fakeWorkspaceRepository{
		withinTransaction: func(gotContext context.Context) {
			if gotContext != ctx {
				t.Fatal("transaction context differs from request context")
			}
		},
		create: func(gotContext context.Context, subject, idempotencyKey, name string) (domain.Workspace, error) {
			if gotContext != ctx {
				t.Fatal("create context differs from request context")
			}
			if subject != "subject-1" || idempotencyKey != "request-1" || name != "Platform" {
				t.Fatalf("unexpected create input: %q, %q, %q", subject, idempotencyKey, name)
			}
			return want, nil
		},
	}

	got, err := NewWorkspaceService(repository).CreateWorkspace(ctx, inbound.CreateWorkspaceInput{
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
	if repository.transactions != 1 {
		t.Fatalf("transactions = %d, want 1", repository.transactions)
	}
}

func TestWorkspaceServiceAcceptsIdempotencyKeyBoundaries(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "one byte", key: "!"},
		{name: "255 bytes", key: strings.Repeat("~", 255)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeWorkspaceRepository{
				create: func(_ context.Context, _, idempotencyKey, _ string) (domain.Workspace, error) {
					if idempotencyKey != tt.key {
						t.Fatalf("idempotency key = %q, want %q", idempotencyKey, tt.key)
					}
					return domain.Workspace{}, nil
				},
			}

			_, err := NewWorkspaceService(repository).CreateWorkspace(context.Background(), inbound.CreateWorkspaceInput{
				Subject:        "subject",
				IdempotencyKey: tt.key,
				Name:           "Workspace",
			})
			if err != nil {
				t.Fatal(err)
			}
		})
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
		{name: "idempotency key contains space", input: inbound.CreateWorkspaceInput{Subject: "subject", IdempotencyKey: "request key", Name: "Workspace"}, want: domain.ErrInvalidArgument},
		{name: "idempotency key contains delete", input: inbound.CreateWorkspaceInput{Subject: "subject", IdempotencyKey: "request\x7fkey", Name: "Workspace"}, want: domain.ErrInvalidArgument},
		{name: "idempotency key contains non-ASCII", input: inbound.CreateWorkspaceInput{Subject: "subject", IdempotencyKey: "request-é", Name: "Workspace"}, want: domain.ErrInvalidArgument},
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
			if repository.transactions != 0 {
				t.Fatalf("transactions = %d, want 0", repository.transactions)
			}
		})
	}
}

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

	got, err := NewWorkspaceService(repository).GetWorkspace(ctx, inbound.GetWorkspaceInput{
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

func TestWorkspaceServicePreservesCreateContextErrors(t *testing.T) {
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
				create: func(context.Context, string, string, string) (domain.Workspace, error) {
					return domain.Workspace{}, tt.want
				},
			}

			_, err := NewWorkspaceService(repository).CreateWorkspace(context.Background(), inbound.CreateWorkspaceInput{
				Subject:        "subject",
				IdempotencyKey: "key",
				Name:           "Workspace",
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
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

			_, err := NewWorkspaceService(repository).GetWorkspace(context.Background(), inbound.GetWorkspaceInput{
				Subject:     "subject",
				WorkspaceID: "workspace-id",
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

type fakeWorkspaceRepository struct {
	withinTransaction func(context.Context)
	create            func(context.Context, string, string, string) (domain.Workspace, error)
	get               func(context.Context, string, string) (domain.Workspace, error)
	transactions      int
}

func (r *fakeWorkspaceRepository) WithinTransaction(ctx context.Context, fn func(outbound.WorkspaceTransaction) error) error {
	r.transactions++
	if r.withinTransaction != nil {
		r.withinTransaction(ctx)
	}
	return fn(fakeWorkspaceTransaction{create: r.create})
}

func (r *fakeWorkspaceRepository) GetWorkspace(ctx context.Context, subject, workspaceID string) (domain.Workspace, error) {
	return r.get(ctx, subject, workspaceID)
}

type fakeWorkspaceTransaction struct {
	create func(context.Context, string, string, string) (domain.Workspace, error)
}

func (tx fakeWorkspaceTransaction) CreateWorkspace(ctx context.Context, subject, idempotencyKey, name string) (domain.Workspace, error) {
	return tx.create(ctx, subject, idempotencyKey, name)
}
