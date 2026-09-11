package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	httptransport "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/keycloak"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/postgres"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/app"
	"google.golang.org/grpc"
)

const (
	serverTimeout       = 5 * time.Second
	idleTimeout         = 30 * time.Second
	maxRequestBodyBytes = 1 << 20
)

type Server struct {
	httpServer *http.Server
	pool       *pgxpool.Pool
}

func NewServer(ctx context.Context, config Config) (*Server, error) {
	pool, err := pgxpool.New(ctx, config.DatabaseURL)
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
	handler, err := newHandler(ctx, httptransport.NewWorkspaceHandler(workspaceService, verifier))
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
		pool:       pool,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	defer s.pool.Close()
	log.Printf("workspace API listening on %s", s.httpServer.Addr)
	return serve(ctx, s.httpServer.ListenAndServe, s.httpServer.Shutdown)
}

func serve(ctx context.Context, listenAndServe func() error, shutdown func(context.Context) error) error {
	shutdownResult := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), serverTimeout)
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

	httpHandler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.ProtoMajor == 2 && strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/grpc") {
			grpcServer.ServeHTTP(response, request)
			return
		}
		gateway.ServeHTTP(response, request)
	})
	return http.MaxBytesHandler(httpHandler, maxRequestBodyBytes), nil
}

func incomingHeader(key string) (string, bool) {
	if strings.EqualFold(key, "Idempotency-Key") {
		return "idempotency-key", true
	}
	return runtime.DefaultHeaderMatcher(key)
}
