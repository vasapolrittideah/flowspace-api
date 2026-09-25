package main

import (
	"testing"

	"go.uber.org/zap"
)

func TestRunStopsOnMissingDeliveryKey(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("DELIVERY_KEY_FILE", "")
	if err := run(zap.NewNop()); err == nil {
		t.Fatal("worker started without a delivery key")
	}
}
