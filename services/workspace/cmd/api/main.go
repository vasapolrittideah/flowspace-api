package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	httptransport "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/keycloak"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/adapter/out/postgres"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/bootstrap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	databaseURL, err := requiredEnvironment("DATABASE_URL")
	if err != nil {
		return err
	}
	issuer, err := requiredEnvironment("OIDC_ISSUER")
	if err != nil {
		return err
	}
	audience, err := requiredEnvironment("OIDC_AUDIENCE")
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("configure database: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	verifier, err := keycloak.NewTokenVerifier(ctx, issuer, audience)
	if err != nil {
		return err
	}
	workspaceService := app.NewWorkspaceService(postgres.NewWorkspaceRepository(pool))
	handler, err := bootstrap.NewServer(ctx, httptransport.NewWorkspaceHandler(workspaceService, verifier))
	if err != nil {
		return fmt.Errorf("configure transport: %w", err)
	}

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{
		Addr:              environmentOr("HTTP_ADDR", ":8080"),
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("workspace API listening on %s", server.Addr)
	return serve(ctx, server.ListenAndServe, server.Shutdown)
}

func serve(ctx context.Context, listenAndServe func() error, shutdown func(context.Context) error) error {
	shutdownResult := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

func requiredEnvironment(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func environmentOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
