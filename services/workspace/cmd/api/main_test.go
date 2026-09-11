package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestServeWaitsForShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	shutdownErr := errors.New("shutdown timed out")

	err := serve(ctx, func() error {
		return http.ErrServerClosed
	}, func(context.Context) error {
		return shutdownErr
	})

	if !errors.Is(err, shutdownErr) {
		t.Fatalf("serve() error = %v, want %v", err, shutdownErr)
	}
}
