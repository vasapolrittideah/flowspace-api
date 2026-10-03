package main

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func TestRunRequiresEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "")

	if err := run(zap.NewNop()); err == nil {
		t.Fatal("run() accepted a missing environment")
	}
}

func TestRunInstallsTracing(t *testing.T) {
	propagator := otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTextMapPropagator(propagator) })
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("ENVIRONMENT", "")

	_ = run(zap.NewNop())

	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}, TraceFlags: trace.FlagsSampled,
	}))
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if carrier.Get("traceparent") == "" {
		t.Fatal("run() did not install the trace context propagator")
	}
}
