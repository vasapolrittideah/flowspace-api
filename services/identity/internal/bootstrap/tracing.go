package bootstrap

import (
	"context"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func StartTracing() func() {
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))))
	otel.SetTracerProvider(provider)
	return func() { _ = provider.Shutdown(context.Background()) }
}
