package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/vasapolrittideah/flowspace-api/internal/logging"
	"github.com/vasapolrittideah/flowspace-api/internal/tracing"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/bootstrap"
)

const service = "workspace-api"

func main() {
	logger := logging.New(service, os.Getenv("ENVIRONMENT"))
	os.Exit(logging.Run(logger, func() error { return run(logger) }))
}

func run(logger *zap.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopTracing, err := tracing.Start(ctx, logger, service, os.Getenv("ENVIRONMENT"))
	if err != nil {
		return err
	}
	defer stopTracing()

	config, err := bootstrap.LoadConfig()
	if err != nil {
		return err
	}
	server, err := bootstrap.NewServer(ctx, config, logger)
	if err != nil {
		return err
	}
	return server.Run(ctx)
}
