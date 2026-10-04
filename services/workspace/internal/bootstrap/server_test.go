package bootstrap

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	"github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1/workspacev1connect"
	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
	"github.com/vasapolrittideah/flowspace-api/internal/requestid"
	httptransport "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/in"
)

func TestNewServerRejectsInvalidDatabaseURL(t *testing.T) {
	if _, err := NewServer(context.Background(), Config{DatabaseURL: "postgres://%"}, zap.NewNop()); err == nil {
		t.Fatal("NewServer() accepted an invalid database URL")
	}
}

func TestNewServerRedactsInvalidDatabaseURL(t *testing.T) {
	const secret = "database-secret"
	_, err := NewServer(context.Background(), Config{DatabaseURL: sharedconfig.Secret("postgres://workspace:" + secret + "@%")}, zap.NewNop())
	if err == nil {
		t.Fatal("NewServer() accepted an invalid database URL")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("NewServer() error contains database credentials: %v", err)
	}
}

func TestServerRunLogsListening(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pool, err := pgxpool.New(ctx, "postgres://workspace@localhost/workspace")
	if err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.InfoLevel)
	server := &Server{
		httpServer: &http.Server{Addr: "localhost:-1", ReadHeaderTimeout: serverTimeout},
		logger:     zap.New(core),
		pool:       pool,
	}

	if err := server.Run(ctx); err == nil {
		t.Fatal("Run() accepted an invalid listen address")
	}
	entry := logs.AllUntimed()[0]
	if entry.Message != "process_listening" || entry.ContextMap()["address"] != "localhost:-1" {
		t.Fatalf("log = %v", entry)
	}
}

func TestHandlerServesREST(t *testing.T) {
	server := &fakeWorkspaceServer{create: func(ctx context.Context, request *workspacev1.CreateWorkspaceRequest) (*workspacev1.CreateWorkspaceResponse, error) {
		if got := metadata.ValueFromIncomingContext(ctx, "authorization"); len(got) != 1 || got[0] != "Bearer token" {
			t.Fatalf("authorization metadata = %v", got)
		}
		if got := metadata.ValueFromIncomingContext(ctx, "idempotency-key"); len(got) != 1 || got[0] != "request-1" {
			t.Fatalf("idempotency metadata = %v", got)
		}
		if got := metadata.ValueFromIncomingContext(ctx, "x-request-id"); len(got) != 1 || got[0] != "request-2" {
			t.Fatalf("request ID metadata = %v", got)
		}
		assertRESTContext(ctx, t)
		return &workspacev1.CreateWorkspaceResponse{Workspace: &workspacev1.Workspace{Id: "workspace-1", Name: request.GetName()}}, nil
	}}
	handler, err := newHandler(context.Background(), server)
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/workspaces", strings.NewReader(`{"name":"Flow Space"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token")
	request.Header.Set("Idempotency-Key", "request-1")
	request.Header.Set("X-Request-ID", "request-2")
	request.Header.Set("Grpc-Timeout", "1S")
	request.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	request.Header.Set("Tracestate", "vendor=value")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	if got := response.Header().Get("X-Request-ID"); got != "request-2" {
		t.Fatalf("X-Request-ID = %q, want request-2", got)
	}
}

func assertRESTContext(ctx context.Context, t *testing.T) {
	t.Helper()
	if got := trace.SpanContextFromContext(ctx); got.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" || got.TraceState().String() != "vendor=value" {
		t.Fatalf("span context = %v", got)
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > time.Second {
		t.Fatalf("deadline = %v, present = %t", deadline, ok)
	}
}

func TestRequestIDValidatesHeader(t *testing.T) {
	if got := requestid.ValidOrNew("request-3"); got != "request-3" {
		t.Fatalf("requestID() = %q, want request-3", got)
	}
	for _, safe := range []string{"a", "Request.ID_3-", strings.Repeat("x", 128)} {
		if got := requestid.ValidOrNew(safe); got != safe {
			t.Fatalf("requestID(%q) = %q", safe, got)
		}
	}
	for _, unsafe := range []string{"", "unsafe request id", "request/id", "คำขอ", strings.Repeat("x", 129)} {
		if got := requestid.ValidOrNew(unsafe); got == unsafe || !requestid.Valid(got) {
			t.Fatalf("requestID(%q) = %q", unsafe, got)
		}
	}
}

func TestHandlerReplacesInvalidRequestID(t *testing.T) {
	var forwarded string
	server := &fakeWorkspaceServer{get: func(ctx context.Context, request *workspacev1.GetWorkspaceRequest) (*workspacev1.GetWorkspaceResponse, error) {
		values := metadata.ValueFromIncomingContext(ctx, "x-request-id")
		if len(values) != 1 {
			t.Fatalf("request ID metadata = %v", values)
		}
		forwarded = values[0]
		return &workspacev1.GetWorkspaceResponse{Workspace: &workspacev1.Workspace{Id: request.GetWorkspaceId()}}, nil
	}}
	handler, err := newHandler(context.Background(), server)
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/workspaces/workspace-1", nil)
	request.Header.Set("X-Request-ID", "unsafe request id")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	if returned := response.Header().Get("X-Request-ID"); returned != forwarded || !requestid.Valid(returned) {
		t.Fatalf("returned request ID = %q, forwarded = %q", returned, forwarded)
	}
}

func TestIncomingHeaderForwardsRequiredHeaders(t *testing.T) {
	if got, ok := incomingHeader("Authorization"); ok || got != "" {
		t.Fatalf("incomingHeader(%q) = %q, %t, want empty, false", "Authorization", got, ok)
	}
	for key, want := range map[string]string{
		"Idempotency-Key": "idempotency-key",
		"X-Request-ID":    "x-request-id",
	} {
		if got, ok := incomingHeader(key); !ok || got != want {
			t.Fatalf("incomingHeader(%q) = %q, %t, want %q, true", key, got, ok, want)
		}
	}
}

func TestHandlerMapsAuthenticationFailureToHTTP(t *testing.T) {
	handler, err := newHandler(context.Background(), httptransport.NewWorkspaceHandler(nil, nil, zap.NewNop()))
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/workspaces/workspace-1", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
}

func TestHandlerRejectsOversizedRESTBody(t *testing.T) {
	called := false
	server := &fakeWorkspaceServer{create: func(context.Context, *workspacev1.CreateWorkspaceRequest) (*workspacev1.CreateWorkspaceResponse, error) {
		called = true
		return &workspacev1.CreateWorkspaceResponse{}, nil
	}}
	handler, err := newHandler(context.Background(), server)
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/workspaces", strings.NewReader(`{"name":"`+strings.Repeat("x", maxRequestBodyBytes)+`"}`))
	request.ContentLength = -1
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code < http.StatusBadRequest {
		t.Fatalf("status = %d, want request rejection", response.Code)
	}
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
	if called {
		t.Fatal("oversized request reached the RPC handler")
	}
}

func TestHandlerRejectsMalformedRESTJSON(t *testing.T) {
	called := false
	server := &fakeWorkspaceServer{create: func(context.Context, *workspacev1.CreateWorkspaceRequest) (*workspacev1.CreateWorkspaceResponse, error) {
		called = true
		return &workspacev1.CreateWorkspaceResponse{}, nil
	}}
	handler, err := newHandler(context.Background(), server)
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/workspaces", strings.NewReader(`{"name":`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("malformed request reached the RPC handler")
	}
}

func TestHandlerServesConnectClientOverGRPC(t *testing.T) {
	server := &fakeWorkspaceServer{get: func(_ context.Context, request *workspacev1.GetWorkspaceRequest) (*workspacev1.GetWorkspaceResponse, error) {
		return &workspacev1.GetWorkspaceResponse{Workspace: &workspacev1.Workspace{Id: request.GetWorkspaceId(), Name: "Flow Space"}}, nil
	}}
	handler, err := newHandler(context.Background(), server)
	if err != nil {
		t.Fatalf("newHandler() error = %v", err)
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

func TestServeWaitsForShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	shutdownErr := errors.New("shutdown timed out")
	listened := false

	err := serve(ctx, func() error {
		listened = true
		return http.ErrServerClosed
	}, func(context.Context) error {
		return shutdownErr
	})

	if !errors.Is(err, shutdownErr) {
		t.Fatalf("serve() error = %v, want %v", err, shutdownErr)
	}
	if listened {
		t.Fatal("serve() started listening after cancellation")
	}
}

func TestServeShutsDownAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	shutdownCalled := make(chan struct{})

	err := serve(ctx, func() error {
		cancel()
		<-shutdownCalled
		return http.ErrServerClosed
	}, func(context.Context) error {
		close(shutdownCalled)
		return nil
	})
	if err != nil {
		t.Fatalf("serve() error = %v", err)
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

const (
	parentTraceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
	parentSpanID      = "00f067aa0ba902b7"
	parentTraceparent = "00-" + parentTraceID + "-" + parentSpanID + "-01"
)

// recordSpans installs a global tracer provider that keeps ended spans in
// memory. Tests that call it must not run in parallel.
func recordSpans(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	return exporter
}

func onlySpan(t *testing.T, exporter *tracetest.InMemoryExporter) tracetest.SpanStub {
	t.Helper()
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	return spans[0]
}

func spanAttributes(span tracetest.SpanStub) map[string]string {
	attributes := map[string]string{}
	for _, item := range span.Attributes {
		attributes[string(item.Key)] = item.Value.String()
	}
	return attributes
}

func grpcRequest(t *testing.T, method string, message proto.Message) *http.Request {
	t.Helper()
	payload, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	length := len(payload)
	if length < 0 || length > math.MaxUint32 {
		t.Fatalf("payload has %d bytes", length)
		return nil
	}
	frame := make([]byte, 5+length)
	binary.BigEndian.PutUint32(frame[1:5], uint32(length))
	copy(frame[5:], payload)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, method, strings.NewReader(string(frame)))
	request.ProtoMajor = 2
	request.Header.Set("Content-Type", "application/grpc")
	request.Header.Set("TE", "trailers")
	return request
}

func workspaceServer(err error) *fakeWorkspaceServer {
	return &fakeWorkspaceServer{
		create: func(context.Context, *workspacev1.CreateWorkspaceRequest) (*workspacev1.CreateWorkspaceResponse, error) {
			if err != nil {
				return nil, err
			}
			return &workspacev1.CreateWorkspaceResponse{Workspace: &workspacev1.Workspace{Id: "workspace-1"}}, nil
		},
		get: func(_ context.Context, request *workspacev1.GetWorkspaceRequest) (*workspacev1.GetWorkspaceResponse, error) {
			if err != nil {
				return nil, err
			}
			return &workspacev1.GetWorkspaceResponse{Workspace: &workspacev1.Workspace{Id: request.GetWorkspaceId()}}, nil
		},
	}
}

func serveRequest(t *testing.T, server workspacev1.WorkspaceServiceServer, request *http.Request) {
	t.Helper()
	handler, err := newHandler(t.Context(), server)
	if err != nil {
		t.Fatal(err)
	}
	handler.ServeHTTP(httptest.NewRecorder(), request)
}

func TestHandlerCreatesOneBoundedServerSpan(t *testing.T) {
	getWorkspace := workspacev1.WorkspaceService_GetWorkspace_FullMethodName
	tests := []struct {
		name      string
		request   func(*testing.T) *http.Request
		wantName  string
		wantAttrs map[string]string
	}{
		{
			name: "REST route with a path parameter",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/workspaces/secret-workspace-id", nil)
			},
			wantName: "GET /v1/workspaces/{workspace_id}",
			wantAttrs: map[string]string{
				"http.request.method": "GET", "http.route": "/v1/workspaces/{workspace_id}", "http.response.status_code": "200",
			},
		},
		{
			name: "REST route without a path parameter",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/workspaces", strings.NewReader(`{"name":"Flow"}`))
			},
			wantName: "POST /v1/workspaces",
			wantAttrs: map[string]string{
				"http.request.method": "POST", "http.route": "/v1/workspaces", "http.response.status_code": "200",
			},
		},
		{
			name: "unknown path",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/secret-path/secret-id", nil)
			},
			wantName:  "GET",
			wantAttrs: map[string]string{"http.request.method": "GET", "http.response.status_code": "404"},
		},
		{
			name: "client-defined method",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return httptest.NewRequestWithContext(t.Context(), "SECRET-METHOD", "/v1/workspaces", nil)
			},
			wantName:  "_OTHER",
			wantAttrs: map[string]string{"http.request.method": "_OTHER", "http.response.status_code": "501"},
		},
		{
			name: "malformed REST body",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/workspaces", strings.NewReader(`{`))
			},
			wantName: "POST /v1/workspaces",
			wantAttrs: map[string]string{
				"http.request.method": "POST", "http.route": "/v1/workspaces", "http.response.status_code": "400",
			},
		},
		{
			name: "known gRPC method",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return grpcRequest(t, getWorkspace, &workspacev1.GetWorkspaceRequest{WorkspaceId: "secret-workspace-id"})
			},
			wantName:  getWorkspace,
			wantAttrs: map[string]string{"rpc.method": strings.TrimPrefix(getWorkspace, "/"), "rpc.grpc.status_code": "0"},
		},
		{
			name: "unknown gRPC method",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return grpcRequest(t, "/flowspace.workspace.v1.WorkspaceService/SecretMethod", &workspacev1.GetWorkspaceRequest{})
			},
			wantName:  "POST",
			wantAttrs: map[string]string{"rpc.grpc.status_code": "12"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exporter := recordSpans(t)
			serveRequest(t, workspaceServer(nil), test.request(t))

			span := onlySpan(t, exporter)
			if span.Name != test.wantName || span.SpanKind != trace.SpanKindServer {
				t.Fatalf("span = %q kind %v, want %q server", span.Name, span.SpanKind, test.wantName)
			}
			if got := spanAttributes(span); fmt.Sprint(got) != fmt.Sprint(test.wantAttrs) {
				t.Fatalf("attributes = %v, want %v", got, test.wantAttrs)
			}
		})
	}
}

func TestHandlerContinuesIncomingTraceparent(t *testing.T) {
	getWorkspace := workspacev1.WorkspaceService_GetWorkspace_FullMethodName
	for name, request := range map[string]func(*testing.T) *http.Request{
		"REST": func(t *testing.T) *http.Request {
			t.Helper()
			return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/workspaces/workspace-1", nil)
		},
		"gRPC": func(t *testing.T) *http.Request {
			t.Helper()
			return grpcRequest(t, getWorkspace, &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			exporter := recordSpans(t)
			var handled trace.SpanContext
			server := workspaceServer(nil)
			get := server.get
			server.get = func(ctx context.Context, request *workspacev1.GetWorkspaceRequest) (*workspacev1.GetWorkspaceResponse, error) {
				handled = trace.SpanContextFromContext(ctx)
				return get(ctx, request)
			}
			incoming := request(t)
			incoming.Header.Set("Traceparent", parentTraceparent)
			incoming.Header.Set("Tracestate", "vendor=value")
			serveRequest(t, server, incoming)

			span := onlySpan(t, exporter)
			if span.Parent.TraceID().String() != parentTraceID || span.Parent.SpanID().String() != parentSpanID || !span.Parent.IsRemote() {
				t.Fatalf("parent = %v", span.Parent)
			}
			if handled.SpanID() != span.SpanContext.SpanID() || handled.IsRemote() || handled.TraceState().String() != "vendor=value" {
				t.Fatalf("handler span context = %v, server span = %v", handled, span.SpanContext)
			}
		})
	}
}

func TestHandlerStartsARootWithoutAValidTraceparent(t *testing.T) {
	for _, traceparent := range []string{"", "not-a-traceparent", "00-" + parentTraceID + "-0000000000000000-01"} {
		exporter := recordSpans(t)
		incoming := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/workspaces/workspace-1", nil)
		if traceparent != "" {
			incoming.Header.Set("Traceparent", traceparent)
		}
		serveRequest(t, workspaceServer(nil), incoming)

		span := onlySpan(t, exporter)
		if span.Parent.IsValid() || !span.SpanContext.IsValid() {
			t.Fatalf("traceparent %q: parent %v, span %v", traceparent, span.Parent, span.SpanContext)
		}
	}
}

func TestHandlerRecordsServerFailures(t *testing.T) {
	getWorkspace := workspacev1.WorkspaceService_GetWorkspace_FullMethodName
	for _, test := range []struct {
		code       codes.Code
		wantError  bool
		wantStatus string
	}{
		{codes.NotFound, false, "404"},
		{codes.Unauthenticated, false, "401"},
		{codes.Internal, true, "500"},
		{codes.Unavailable, true, "503"},
		{codes.DeadlineExceeded, true, "504"},
		{codes.Unknown, true, "500"},
	} {
		for name, request := range map[string]func(*testing.T) *http.Request{
			"REST": func(t *testing.T) *http.Request {
				t.Helper()
				return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/workspaces/workspace-1", nil)
			},
			"gRPC": func(t *testing.T) *http.Request {
				t.Helper()
				return grpcRequest(t, getWorkspace, &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
			},
		} {
			exporter := recordSpans(t)
			serveRequest(t, workspaceServer(status.Error(test.code, "secret-detail")), request(t))

			span := onlySpan(t, exporter)
			if got := span.Status.Code == otelcodes.Error; got != test.wantError || span.Status.Description != "" {
				t.Fatalf("%s %v: span status = %v", name, test.code, span.Status)
			}
			attributes := spanAttributes(span)
			if name == "REST" && attributes["http.response.status_code"] != test.wantStatus ||
				name == "gRPC" && attributes["rpc.grpc.status_code"] != strconv.Itoa(int(test.code)) {
				t.Fatalf("%s %v: attributes = %v", name, test.code, attributes)
			}
		}
	}
}

func TestHandlerCorrelatesTheCompletionLine(t *testing.T) {
	getWorkspace := workspacev1.WorkspaceService_GetWorkspace_FullMethodName
	for name, request := range map[string]func(*testing.T) *http.Request{
		"REST": func(t *testing.T) *http.Request {
			t.Helper()
			return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/workspaces/workspace-1", nil)
		},
		"gRPC": func(t *testing.T) *http.Request {
			t.Helper()
			return grpcRequest(t, getWorkspace, &workspacev1.GetWorkspaceRequest{WorkspaceId: "workspace-1"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			exporter := recordSpans(t)
			core, logs := observer.New(zap.InfoLevel)
			service := &fakeService{get: func(context.Context, inbound.GetWorkspaceInput) (domain.Workspace, error) {
				return domain.Workspace{ID: "workspace-1"}, nil
			}}
			handler := httptransport.NewWorkspaceHandler(service, fakeVerifier{subject: "secret-subject"}, zap.New(core))
			incoming := request(t)
			incoming.Header.Set("Authorization", "Bearer secret-access-token")
			incoming.Header.Set("Cookie", "session=secret-cookie")
			incoming.Header.Set("X-Request-ID", "request-1")
			serveRequest(t, handler, incoming)

			span := onlySpan(t, exporter)
			entries := logs.FilterMessage("request_completed").All()
			if len(entries) != 1 {
				t.Fatalf("request_completed lines = %d", len(entries))
			}
			fields := entries[0].ContextMap()
			if fields["request_id"] != "request-1" || fields["trace_id"] != span.SpanContext.TraceID().String() ||
				fields["operation"] != getWorkspace || fields["outcome"] != "success" || fields["status"] != "OK" || fields["duration"] == nil {
				t.Fatalf("request_completed = %v", fields)
			}
			telemetry := fmt.Sprint(span.Name, span.Attributes, span.Status, span.Events, fields)
			if strings.Contains(telemetry, "secret-") {
				t.Fatalf("telemetry contains request data: %s", telemetry)
			}
		})
	}
}

type fakeService struct {
	inbound.WorkspaceService
	get func(context.Context, inbound.GetWorkspaceInput) (domain.Workspace, error)
}

func (s *fakeService) GetWorkspace(ctx context.Context, input inbound.GetWorkspaceInput) (domain.Workspace, error) {
	return s.get(ctx, input)
}

type fakeVerifier struct{ subject string }

func (v fakeVerifier) VerifyToken(context.Context, string) (string, error) { return v.subject, nil }
