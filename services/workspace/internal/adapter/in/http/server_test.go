package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	"github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1/workspacev1connect"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
)

func TestServerHandlerServesAuthenticatedREST(t *testing.T) {
	var gotInput inbound.CreateWorkspaceInput
	workspaceHandler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{create: func(_ context.Context, input inbound.CreateWorkspaceInput) (domain.Workspace, error) {
			gotInput = input
			return domain.Workspace{ID: "workspace-1", Name: input.Name}, nil
		}},
		fakeTokenVerifier{subject: "user-1"},
	)
	handler, err := NewServerHandler(context.Background(), workspaceHandler)
	if err != nil {
		t.Fatalf("NewServerHandler() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/workspaces", strings.NewReader(`{"name":"Flow Space"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("Idempotency-Key", "request-1")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	if gotInput != (inbound.CreateWorkspaceInput{Subject: "user-1", IdempotencyKey: "request-1", Name: "Flow Space"}) {
		t.Fatalf("input = %+v", gotInput)
	}
}

func TestServerHandlerMapsAuthenticationFailureToHTTP(t *testing.T) {
	handler, err := NewServerHandler(context.Background(), NewWorkspaceHandler(&fakeWorkspaceUsecase{}, fakeTokenVerifier{subject: "user-1"}))
	if err != nil {
		t.Fatalf("NewServerHandler() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/workspaces/workspace-1", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
}

func TestServerHandlerServesConnectClientOverGRPC(t *testing.T) {
	workspaceHandler := NewWorkspaceHandler(
		&fakeWorkspaceUsecase{get: func(_ context.Context, input inbound.GetWorkspaceInput) (domain.Workspace, error) {
			return domain.Workspace{ID: input.WorkspaceID, Name: "Flow Space"}, nil
		}},
		fakeTokenVerifier{subject: "user-1"},
	)
	handler, err := NewServerHandler(context.Background(), workspaceHandler)
	if err != nil {
		t.Fatalf("NewServerHandler() error = %v", err)
	}
	client := workspacev1connect.NewWorkspaceServiceClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		request.Proto = "HTTP/2.0"
		request.ProtoMajor = 2
		request.ProtoMinor = 0
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Result(), nil
	})}, "https://workspace.test", connect.WithGRPC())
	request := connect.NewRequest(&workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
	request.Header().Set("Authorization", "Bearer token")

	response, err := client.GetWorkspace(context.Background(), request)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if got := response.Msg.GetWorkspace(); got.GetId() != "workspace-1" || got.GetName() != "Flow Space" {
		t.Fatalf("workspace = %+v", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
