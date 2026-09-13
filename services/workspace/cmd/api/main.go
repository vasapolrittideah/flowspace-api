package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/vasapolrittideah/flowspace-api/internal/logging"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/bootstrap"
)

func main() {
	logger := logging.New("workspace-api", os.Getenv("ENVIRONMENT"))
	os.Exit(logging.Run(logger, func() error { return run(logger) }))
}

func run(logger *zap.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
