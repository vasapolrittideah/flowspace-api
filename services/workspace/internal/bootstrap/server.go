package bootstrap

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	httptransport "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/keycloak"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/postgres"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/app"
)

const (
	serverTimeout       = 5 * time.Second
	idleTimeout         = 30 * time.Second
	maxRequestBodyBytes = 1 << 20
	maxRequestIDBytes   = 128
)

type Server struct {
	httpServer *http.Server
	logger     *zap.Logger
	pool       *pgxpool.Pool
}

func NewServer(ctx context.Context, config Config, logger *zap.Logger) (*Server, error) {
	pool, err := pgxpool.New(ctx, string(config.DatabaseURL))
	if err != nil {
		return nil, fmt.Errorf("configure database: %w", err)
	}
	keepPool := false
	defer func() {
		if !keepPool {
			pool.Close()
		}
	}()
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	verifier, err := keycloak.NewTokenVerifier(ctx, config.OIDCDiscoveryURL, config.OIDCIssuer, config.OIDCAudience)
	if err != nil {
		return nil, fmt.Errorf("configure authentication: %w", err)
	}
	workspaceService := app.NewWorkspaceService(postgres.NewWorkspaceRepository(pool))
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
	return &Server{
		httpServer: httpServer,
		logger:     logger,
		pool:       pool,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	defer s.pool.Close()
	s.logger.Info("process_listening", zap.String("address", s.httpServer.Addr))
	return serve(ctx, s.httpServer.ListenAndServe, s.httpServer.Shutdown)
}

func serve(ctx context.Context, listenAndServe func() error, shutdown func(context.Context) error) error {
	shutdownResult := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), serverTimeout)
		defer cancel()
		shutdownResult <- shutdown(shutdownCtx)
	}()

	if err := listenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve workspace API: %w", err)
	}
	if ctx.Err() != nil {
		if err := <-shutdownResult; err != nil {
			return fmt.Errorf("shut down workspace API: %w", err)
		}
	}
	return nil
}

func newHandler(ctx context.Context, handler workspacev1.WorkspaceServiceServer) (http.Handler, error) {
	grpcServer := grpc.NewServer()
	workspacev1.RegisterWorkspaceServiceServer(grpcServer, handler)

	gateway := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(incomingHeader))
	if err := workspacev1.RegisterWorkspaceServiceHandlerServer(ctx, gateway, handler); err != nil {
		return nil, err
	}
	restHandler := withBodyLimit(gateway)

	httpHandler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.ProtoMajor == 2 && strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/grpc") {
			grpcServer.ServeHTTP(response, request)
			return
		}
		restHandler.ServeHTTP(response, request)
	})
	return withRequestID(withTraceContext(httpHandler)), nil
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

func withTraceContext(next http.Handler) http.Handler {
	propagator := propagation.TraceContext{}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		ctx := propagator.Extract(request.Context(), propagation.HeaderCarrier(request.Header))
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		id := requestID(request.Header.Get("X-Request-ID"))
		request.Header.Set("X-Request-ID", id)
		response.Header().Set("X-Request-ID", id)
		next.ServeHTTP(response, request)
	})
}

func requestID(value string) string {
	if validRequestID(value) {
		return value
	}
	return rand.Text()
}

func validRequestID(value string) bool {
	if value == "" || len(value) > maxRequestIDBytes {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
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
