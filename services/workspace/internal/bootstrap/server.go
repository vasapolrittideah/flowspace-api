package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	"github.com/vasapolrittideah/flowspace-api/internal/metrics"
	"github.com/vasapolrittideah/flowspace-api/internal/postgrespool"
	"github.com/vasapolrittideah/flowspace-api/internal/requestid"
	"github.com/vasapolrittideah/flowspace-api/internal/tracing"
	httptransport "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/identity"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/postgres"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/app"
)

const (
	serverTimeout       = 5 * time.Second
	idleTimeout         = 30 * time.Second
	maxRequestBodyBytes = 1 << 20
)

type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
	pool       *postgrespool.Pool
	verifier   *identity.TokenVerifier
}

func NewServer(ctx context.Context, config Config, logger *zap.Logger) (*Server, error) {
	if config.OIDCDiscoveryURL != "" || config.OIDCIssuer != "" || config.OIDCAudience != "" {
		return nil, errors.New("OIDC configuration is no longer supported")
	}
	pool, err := postgrespool.Open(ctx, string(config.DatabaseURL))
	if errors.Is(err, postgrespool.ErrInvalidConfiguration) {
		return nil, fmt.Errorf("configure database: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	keepPool := false
	defer func() {
		if !keepPool {
			pool.Close()
		}
	}()

	verifier, err := identity.NewTokenVerifier(identity.Config{
		JWKSURL: config.IdentityJWKSURL, Issuer: config.IdentityIssuer, Audience: config.IdentityAudience,
		SessionAddress: config.IdentitySessionAddress, SessionServerName: config.IdentitySessionServerName,
		ClientCertFile: config.IdentityClientCertFile, ClientKeyFile: config.IdentityClientKeyFile, CAFile: config.IdentityCAFile,
	})
	if err != nil {
		return nil, fmt.Errorf("configure authentication: %w", err)
	}
	keepVerifier := false
	defer func() {
		if !keepVerifier {
			_ = verifier.Close()
		}
	}()
	workspaceRepository := postgres.NewWorkspaceRepository(pool.Pool)
	workspaceService := app.NewWorkspaceService(workspaceRepository)
	handler, err := newHandler(ctx, httptransport.NewWorkspaceHandler(workspaceService, verifier, logger))
	if err != nil {
		return nil, fmt.Errorf("configure transport: %w", err)
	}

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	httpServer := &http.Server{
		Addr:              config.HTTPAddress,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: serverTimeout,
		ReadTimeout:       serverTimeout,
		WriteTimeout:      serverTimeout,
		IdleTimeout:       idleTimeout,
	}
	keepPool = true
	keepVerifier = true
	return &Server{
		httpServer: httpServer,
		logger:     logger,
		pool:       pool,
		verifier:   verifier,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	defer s.pool.Close()
	if s.verifier != nil {
		defer func() { _ = s.verifier.Close() }()
	}
	s.logger.Info("process_listening", zap.String("address", s.httpServer.Addr))
	return serve(ctx, s.httpServer.ListenAndServe, s.httpServer.Shutdown)
}

func serve(ctx context.Context, listenAndServe func() error, shutdown func(context.Context) error) error {
	if ctx.Err() != nil {
		return shutDown(ctx, shutdown)
	}
	shutdownResult := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownResult <- shutDown(ctx, shutdown)
	}()

	if err := listenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve workspace API: %w", err)
	}
	if ctx.Err() != nil {
		return <-shutdownResult
	}
	return nil
}

func shutDown(ctx context.Context, shutdown func(context.Context) error) error {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), serverTimeout)
	defer cancel()
	if err := shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down workspace API: %w", err)
	}
	return nil
}

func newHandler(ctx context.Context, handler workspacev1.WorkspaceServiceServer) (http.Handler, error) {
	grpcServer := grpc.NewServer()
	workspacev1.RegisterWorkspaceServiceServer(grpcServer, handler)
	grpcMethods := map[string]bool{}
	for service, info := range grpcServer.GetServiceInfo() {
		for _, method := range info.Methods {
			grpcMethods["/"+service+"/"+method.Name] = true
		}
	}

	gateway := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(incomingHeader), runtime.WithMiddlewares(recordRoute))
	if err := workspacev1.RegisterWorkspaceServiceHandlerServer(ctx, gateway, handler); err != nil {
		return nil, err
	}
	restHandler := withBodyLimit(gateway)

	httpHandler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if isGRPCRequest(request) {
			grpcServer.ServeHTTP(response, request)
			return
		}
		restHandler.ServeHTTP(response, request)
	})
	return withRequestID(withServerSpan(httpHandler, grpcMethods)), nil
}

func isGRPCRequest(request *http.Request) bool {
	return request.ProtoMajor == 2 && strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/grpc")
}

func withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Body == nil {
			next.ServeHTTP(response, request)
			return
		}
		body := request.Body
		defer func() { _ = body.Close() }()
		content, err := io.ReadAll(http.MaxBytesReader(response, body, maxRequestBodyBytes))
		if err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				http.Error(response, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(response, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(content))
		next.ServeHTTP(response, request)
	})
}

// matchedRoute holds the route template that recordRoute finds for a REST
// request, so that withServerSpan can record it with the duration.
type matchedRoute struct{}

// withServerSpan creates the server span and records the duration of each
// public request. A known gRPC method names the span. Otherwise the span
// takes the HTTP method, and recordRoute adds the route of a matched REST
// request. grpcMethods holds the full names of the gRPC methods that the
// server serves.
func withServerSpan(next http.Handler, grpcMethods map[string]bool) http.Handler {
	propagator := propagation.TraceContext{}
	durations := metrics.NewServerDurations(otel.Meter("flowspace/workspace/api"))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		started := time.Now()
		grpcRequest := isGRPCRequest(request)
		name := tracing.HTTPMethod(request.Method)
		attributes := []attribute.KeyValue{semconv.HTTPRequestMethodKey.String(name)}
		if grpcRequest {
			attributes = nil
			if grpcMethods[request.URL.Path] {
				name = request.URL.Path
				attributes = append(attributes, semconv.RPCMethod(strings.TrimPrefix(name, "/")))
			}
		}
		ctx := propagator.Extract(request.Context(), propagation.HeaderCarrier(request.Header))
		ctx, span := otel.Tracer("flowspace/workspace/api").Start(ctx, name,
			trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attributes...))
		defer span.End()
		route := new(string)
		ctx = context.WithValue(ctx, matchedRoute{}, route)
		observed := &statusWriter{ResponseWriter: response, status: http.StatusOK}
		next.ServeHTTP(observed, request.WithContext(ctx))
		elapsed := time.Since(started)
		if code, ok := tracing.GRPCStatus(observed.Header()); grpcRequest && ok {
			tracing.SetGRPCStatus(span, code)
			method := ""
			if grpcMethods[request.URL.Path] {
				method = request.URL.Path
			}
			durations.RecordRPC(ctx, elapsed, method, code)
			return
		}
		tracing.SetHTTPStatus(span, observed.status)
		durations.RecordHTTP(ctx, elapsed, request.Method, *route, observed.status)
	})
}

// recordRoute names the server span after the route template of the matched
// gateway route, such as GET /v1/workspaces/{workspace_id}, and keeps the
// route for the duration.
func recordRoute(next runtime.HandlerFunc) runtime.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request, parameters map[string]string) {
		if pattern, ok := runtime.HTTPPattern(request.Context()); ok {
			route := strings.ReplaceAll(pattern.String(), "=*}", "}")
			span := trace.SpanFromContext(request.Context())
			span.SetName(tracing.HTTPMethod(request.Method) + " " + route)
			span.SetAttributes(semconv.HTTPRoute(route))
			if matched, ok := request.Context().Value(matchedRoute{}).(*string); ok {
				*matched = route
			}
		}
		next(response, request, parameters)
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		id := requestid.ValidOrNew(request.Header.Get("X-Request-ID"))
		request.Header.Set("X-Request-ID", id)
		response.Header().Set("X-Request-ID", id)
		next.ServeHTTP(response, request)
	})
}

func incomingHeader(key string) (string, bool) {
	switch {
	case strings.EqualFold(key, "Authorization"):
		// grpc-gateway forwards Authorization before it calls this matcher.
		return "", false
	case strings.EqualFold(key, "Idempotency-Key"):
		return "idempotency-key", true
	case strings.EqualFold(key, "X-Request-ID"):
		return "x-request-id", true
	}
	return runtime.DefaultHeaderMatcher(key)
}
