package main

import "testing"

func TestMigrateRequiresEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "")

	if err := migrate(); err == nil {
		t.Fatal("migrate() accepted a missing environment")
	}
}
