package main

import (
	"testing"

	"go.uber.org/zap"
)

func TestRunStopsOnMissingKeys(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("SIGNING_KEY_FILE", "")
	if err := run(zap.NewNop()); err == nil {
		t.Fatal("API started without a signing key")
	}
}
