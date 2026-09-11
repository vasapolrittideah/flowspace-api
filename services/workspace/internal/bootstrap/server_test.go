package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	"github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1/workspacev1connect"
	httptransport "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/in/http"
	"google.golang.org/grpc/metadata"
)

func TestServerHandlerServesREST(t *testing.T) {
	server := &fakeWorkspaceServer{create: func(ctx context.Context, request *workspacev1.CreateWorkspaceRequest) (*workspacev1.CreateWorkspaceResponse, error) {
		if got := metadata.ValueFromIncomingContext(ctx, "authorization"); len(got) != 1 || got[0] != "Bearer token" {
			t.Fatalf("authorization metadata = %v", got)
		}
		if got := metadata.ValueFromIncomingContext(ctx, "idempotency-key"); len(got) != 1 || got[0] != "request-1" {
			t.Fatalf("idempotency metadata = %v", got)
		}
		return &workspacev1.CreateWorkspaceResponse{Workspace: &workspacev1.Workspace{Id: "workspace-1", Name: request.GetName()}}, nil
	}}
	handler, err := NewServer(context.Background(), server)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
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
}

func TestServerHandlerMapsAuthenticationFailureToHTTP(t *testing.T) {
	handler, err := NewServer(context.Background(), httptransport.NewWorkspaceHandler(nil, nil))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/workspaces/workspace-1", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
}

func TestServerHandlerServesConnectClientOverGRPC(t *testing.T) {
	server := &fakeWorkspaceServer{get: func(_ context.Context, request *workspacev1.GetWorkspaceRequest) (*workspacev1.GetWorkspaceResponse, error) {
		return &workspacev1.GetWorkspaceResponse{Workspace: &workspacev1.Workspace{Id: request.GetWorkspaceId(), Name: "Flow Space"}}, nil
	}}
	handler, err := NewServer(context.Background(), server)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
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

	response, err := client.GetWorkspace(context.Background(), request)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if got := response.Msg.GetWorkspace(); got.GetId() != "workspace-1" || got.GetName() != "Flow Space" {
		t.Fatalf("workspace = %+v", got)
	}
}

type fakeWorkspaceServer struct {
	workspacev1.UnimplementedWorkspaceServiceServer
	create func(context.Context, *workspacev1.CreateWorkspaceRequest) (*workspacev1.CreateWorkspaceResponse, error)
	get    func(context.Context, *workspacev1.GetWorkspaceRequest) (*workspacev1.GetWorkspaceResponse, error)
}

func (f *fakeWorkspaceServer) CreateWorkspace(ctx context.Context, request *workspacev1.CreateWorkspaceRequest) (*workspacev1.CreateWorkspaceResponse, error) {
	return f.create(ctx, request)
}

func (f *fakeWorkspaceServer) GetWorkspace(ctx context.Context, request *workspacev1.GetWorkspaceRequest) (*workspacev1.GetWorkspaceResponse, error) {
	return f.get(ctx, request)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
