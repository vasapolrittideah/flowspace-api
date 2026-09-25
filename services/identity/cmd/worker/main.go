package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/vasapolrittideah/flowspace-api/internal/logging"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/bootstrap"
)

func main() {
	logger := logging.New("identity-worker", os.Getenv("ENVIRONMENT"))
	stopTracing := bootstrap.StartTracing()
	code := logging.Run(logger, func() error { return run(logger) })
	stopTracing()
	os.Exit(code)
}

func run(logger *zap.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	config, err := bootstrap.LoadWorkerConfig()
	if err != nil {
		return err
	}
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	worker, err := bootstrap.NewWorker(startupCtx, config, logger)
	if err != nil {
		return err
	}
	return worker.Run(ctx)
}
