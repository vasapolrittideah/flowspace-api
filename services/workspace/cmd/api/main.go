package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/internal/bootstrap"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config, err := bootstrap.LoadConfig()
	if err != nil {
		return err
	}
	server, err := bootstrap.NewServer(ctx, config)
	if err != nil {
		return err
	}
	return server.Run(ctx)
}
