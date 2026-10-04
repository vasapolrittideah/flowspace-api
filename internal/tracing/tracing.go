// Package tracing sets up bounded, non-blocking span export for Flowspace
// processes and records the status of server spans.
package tracing

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.uber.org/zap"
)

const (
	// maxQueueSize and exportTimeout are the limits that ADR-0037 sets.
	maxQueueSize  = 2048
	exportTimeout = 10 * time.Second
	// flushTimeout keeps the 5-second server shutdown and the span flush
	// inside the 10-second termination grace period of each pod.
	flushTimeout = 4 * time.Second
)

// Start installs the global tracer provider and the W3C trace context
// propagator. The SDK reads the endpoint and the sampler from the standard
// OTEL_* variables. Without OTEL_EXPORTER_OTLP_ENDPOINT, spans keep their
// trace context but are not exported. The returned function flushes the
// queued spans for at most four seconds and stops the provider.
func Start(ctx context.Context, logger *zap.Logger, service, environment string) func() {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Warn("tracing_error", zap.Error(err))
	}))
	provider := newProvider(newExporter(ctx), service, environment)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return func() { flush(context.WithoutCancel(ctx), provider, flushTimeout) }
}

// newExporter returns nil when no endpoint is set or the exporter cannot
// start, because telemetry never stops a process. The gRPC connection is
// lazy, so an unreachable endpoint does not delay startup.
func newExporter(ctx context.Context) sdktrace.SpanExporter {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return nil
	}
	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		otel.Handle(err)
		return nil
	}
	return exporter
}

func newProvider(exporter sdktrace.SpanExporter, service, environment string) *sdktrace.TracerProvider {
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource.NewSchemaless(
			semconv.ServiceName(service),
			semconv.DeploymentEnvironmentNameKey.String(environment),
		)),
	}
	if exporter != nil {
		// The batch processor drops new spans when its queue is full, so a
		// request never waits for an export.
		options = append(options, sdktrace.WithBatcher(exporter,
			sdktrace.WithMaxQueueSize(maxQueueSize),
			sdktrace.WithExportTimeout(exportTimeout),
		))
	}
	return sdktrace.NewTracerProvider(options...)
}

// flush drops the spans that it cannot export before the timeout.
func flush(ctx context.Context, provider *sdktrace.TracerProvider, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_ = provider.Shutdown(ctx)
}
