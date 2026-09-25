package bootstrap

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestTracingCreatesPropagatedContext(t *testing.T) {
	previous := otel.GetTracerProvider()
	stop := StartTracing()
	t.Cleanup(func() {
		stop()
		otel.SetTracerProvider(previous)
	})
	ctx, span := otel.Tracer("identity-test").Start(context.Background(), "signup")
	defer span.End()
	carrier := propagation.MapCarrier{}
	(propagation.TraceContext{}).Inject(ctx, carrier)
	if carrier.Get("traceparent") == "" || traceID(ctx) == "" {
		t.Fatal("trace context was not created and propagated")
	}
}
