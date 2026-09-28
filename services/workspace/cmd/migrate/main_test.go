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

func TestMigrateRequiresDatabaseConfiguration(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "")
	for _, key := range []string{"PGHOST", "PGDATABASE", "PGUSER", "PGPASSWORD"} {
		t.Setenv(key, "")
	}

	err := migrate()
	if err == nil {
		t.Fatal("migrate() accepted a missing database URL")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("migrate() error = %v, want DATABASE_URL configuration error", err)
	}
}

func TestMigrateUsesPostgresEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PGHOST", "127.0.0.1")
	t.Setenv("PGPORT", "1")
	t.Setenv("PGDATABASE", "workspace")
	t.Setenv("PGUSER", "workspace")
	t.Setenv("PGPASSWORD", "test-password")
	t.Setenv("PGSSLMODE", "disable")

	err := migrate()
	if err == nil {
		t.Fatal("migrate() accepted an unavailable PostgreSQL server")
	}
	if !strings.Contains(err.Error(), "migrate workspace database") {
		t.Fatalf("migrate() error = %v, want migration error", err)
	}
}

func TestMigrateUsesConfiguredDatabaseURL(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "%")

	err := migrate()
	if err == nil {
		t.Fatal("migrate() accepted an invalid database URL")
	}
	if !strings.Contains(err.Error(), "migrate workspace database") {
		t.Fatalf("migrate() error = %v, want migration error", err)
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
