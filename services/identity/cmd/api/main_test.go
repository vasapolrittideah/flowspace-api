package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func TestRunStopsOnMissingKeys(t *testing.T) {
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("SIGNING_KEY_FILE", "")
	if err := run(zap.NewNop()); err == nil {
		t.Fatal("API started without a signing key")
	}
}

func TestRunInstallsTracing(t *testing.T) {
	propagator := otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTextMapPropagator(propagator) })
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("SIGNING_KEY_FILE", "")

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

func TestRunExportsMetricsAndStopsBothProvidersInTime(t *testing.T) {
	meterProvider, tracerProvider, propagator := otel.GetMeterProvider(), otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetMeterProvider(meterProvider)
		otel.SetTracerProvider(tracerProvider)
		otel.SetTextMapPropagator(propagator)
	})
	otel.SetMeterProvider(noop.NewMeterProvider())
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	t.Setenv("ENVIRONMENT", "local")
	t.Setenv("SIGNING_KEY_FILE", "")
	started := time.Now()

	if err := run(zap.NewNop()); err == nil {
		t.Fatal("run() started without its configuration")
	}

	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("run() started and stopped in %s", elapsed)
	}
	installed, ok := otel.GetMeterProvider().(*sdkmetric.MeterProvider)
	if !ok {
		t.Fatalf("run() installed meter provider %T", otel.GetMeterProvider())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := installed.ForceFlush(ctx); !errors.Is(err, sdkmetric.ErrReaderShutdown) {
		t.Fatalf("metric reader after run() = %v, want it shut down", err)
	}
	if _, span := otel.Tracer("test").Start(ctx, "test"); span.IsRecording() {
		t.Fatal("tracer provider still records spans after run()")
	}
}
