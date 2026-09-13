package main

import (
	"testing"

	"go.uber.org/zap"
)

func TestRunRequiresEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "")

	if err := run(zap.NewNop()); err == nil {
		t.Fatal("run() accepted a missing environment")
	}
}
