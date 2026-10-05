//go:build smoke

// Package alloy_test proves the memory limiter of the local Alloy. Run it with
// scripts/smoke-alloy-memory-local.sh, which stops Tempo and forwards the Alloy
// OTLP port.
package alloy_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestAlloyRefusesSpansAboveTheMemoryLimit sends large spans until Alloy refuses
// them because of its memory limiter. Without the limiter, Alloy accepts spans
// until its exporter queue fills its memory.
func TestAlloyRefusesSpansAboveTheMemoryLimit(t *testing.T) {
	endpoint := os.Getenv("ALLOY_OTLP_ENDPOINT")
	if endpoint == "" {
		t.Fatal("ALLOY_OTLP_ENDPOINT is not set")
	}
	var refused atomic.Bool
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		if strings.Contains(err.Error(), "high memory usage") {
			refused.Store(true)
		}
	}))
	ctx := context.Background()
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter,
		sdktrace.WithMaxQueueSize(200000), sdktrace.WithMaxExportBatchSize(512), sdktrace.WithBatchTimeout(50*time.Millisecond)))
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_ = provider.Shutdown(shutdownCtx)
	})
	tracer := provider.Tracer("alloy-memory-smoke")
	payload := strings.Repeat("x", 1024)
	deadline := time.Now().Add(120 * time.Second)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !refused.Load() && time.Now().Before(deadline) {
				_, span := tracer.Start(ctx, "load")
				span.SetAttributes(attribute.String("payload", payload))
				span.End()
			}
		}()
	}
	wg.Wait()
	if !refused.Load() {
		t.Fatal("Alloy gave no memory limit refusal within 120 seconds")
	}
}
