package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMigrateRequiresEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "")

	if err := migrate(); err == nil {
		t.Fatal("migrate() accepted a missing environment")
	}
}

func TestMigrateRequiresDatabaseURL(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "")

	err := migrate()
	if err == nil {
		t.Fatal("migrate() accepted a missing database URL")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("migrate() error = %v, want DATABASE_URL configuration error", err)
	}
}

func TestRunHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := run(ctx, "postgres://workspace@127.0.0.1:1/workspace")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want %v", err, context.Canceled)
	}
}
