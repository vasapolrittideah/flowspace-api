package tracing

import (
	"context"
	"slices"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

const testTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestMetadataCarrierInjectsAndExtractsTraceContext(t *testing.T) {
	propagator := propagation.TraceContext{}
	incoming := metadata.Pairs("traceparent", testTraceparent, "tracestate", "vendor=value")
	ctx := propagator.Extract(context.Background(), MetadataCarrier(incoming))
	extracted := trace.SpanContextFromContext(ctx)
	if !extracted.IsRemote() || extracted.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" ||
		extracted.SpanID().String() != "00f067aa0ba902b7" || extracted.TraceState().String() != "vendor=value" {
		t.Fatalf("extracted span context = %+v", extracted)
	}
	outgoing := metadata.MD{}
	propagator.Inject(ctx, MetadataCarrier(outgoing))
	if got := outgoing.Get("traceparent"); !slices.Equal(got, []string{testTraceparent}) {
		t.Fatalf("injected traceparent = %v", got)
	}
	if got := outgoing.Get("tracestate"); !slices.Equal(got, []string{"vendor=value"}) {
		t.Fatalf("injected tracestate = %v", got)
	}
	if keys := MetadataCarrier(outgoing).Keys(); !slices.Equal(slices.Sorted(slices.Values(keys)), []string{"traceparent", "tracestate"}) {
		t.Fatalf("keys = %v", keys)
	}
}

func TestMetadataCarrierOmitsAnEmptyTraceState(t *testing.T) {
	propagator := propagation.TraceContext{}
	ctx := propagator.Extract(context.Background(), MetadataCarrier(metadata.Pairs("traceparent", testTraceparent)))
	outgoing := metadata.MD{}
	propagator.Inject(ctx, MetadataCarrier(outgoing))
	if _, ok := outgoing["tracestate"]; ok || len(outgoing.Get("traceparent")) != 1 {
		t.Fatalf("outgoing metadata = %v", outgoing)
	}
}

func TestMetadataCarrierIgnoresRepeatedTraceparents(t *testing.T) {
	incoming := metadata.Pairs("traceparent", testTraceparent, "traceparent", testTraceparent)
	ctx := propagation.TraceContext{}.Extract(context.Background(), MetadataCarrier(incoming))
	if trace.SpanContextFromContext(ctx).IsValid() {
		t.Fatal("repeated traceparent values made a parent")
	}
}
