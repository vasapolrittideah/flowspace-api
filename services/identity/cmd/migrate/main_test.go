package main

import (
	"context"
	"testing"
)

func TestMigrateRequiresDatabase(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DATABASE_URL", "")
	if err := migrate(); err == nil {
		t.Fatal("migration started without a database")
	}
	if err := run(context.Background(), "invalid-dsn"); err == nil {
		t.Fatal("migration accepted an invalid database URL")
	}
}
