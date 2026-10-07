package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/vasapolrittideah/flowspace-api/internal/logging"
	"github.com/vasapolrittideah/flowspace-api/internal/metrics"
	"github.com/vasapolrittideah/flowspace-api/internal/tracing"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/bootstrap"
)

const service = "identity-api"

func main() {
	logger := logging.New(service, os.Getenv("ENVIRONMENT"))
	os.Exit(logging.Run(logger, func() error { return run(logger) }))
}

func run(logger *zap.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopTracing := tracing.Start(ctx, logger, service, os.Getenv("ENVIRONMENT"))
	stopTelemetry := metrics.Start(ctx, service, os.Getenv("ENVIRONMENT"), stopTracing)
	defer stopTelemetry()
	config, err := bootstrap.LoadAPIConfig()
	if err != nil {
		return err
	}
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	server, err := bootstrap.NewAPIServer(startupCtx, config, logger)
	if err != nil {
		return err
	}
	return server.Run(ctx)
}
