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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config, err := bootstrap.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	server, err := bootstrap.NewServer(ctx, config)
	if err != nil {
		log.Fatal(err)
	}
	if err := server.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
