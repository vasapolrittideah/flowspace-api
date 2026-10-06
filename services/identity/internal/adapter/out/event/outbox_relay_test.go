package event_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/event"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type relayRepository struct {
	request   outbound.OutboxEvent
	pending   bool
	published bool
	released  bool
	claimErr  error
}

func (r *relayRepository) Claim(context.Context, string) (outbound.OutboxEvent, bool, error) {
	return r.request, r.pending, r.claimErr
}

func (r *relayRepository) MarkPublished(context.Context, string, string) error {
	r.published = true
	r.pending = false
	return nil
}

func (r *relayRepository) Release(_ context.Context, _, _ string, next time.Time) error {
	r.released = !next.IsZero()
	return nil
}

func TestOutboxRelayRetriesTheSameEventAfterPublishFailure(t *testing.T) {
	request := outbound.OutboxEvent{ID: "event-1", ChallengeID: "challenge-1", Purpose: "verify-email"}
	repository := &relayRepository{request: request, pending: true}
	var published []outbound.OutboxEvent
	relay := event.NewOutboxRelay(repository, func(_ context.Context, got outbound.OutboxEvent) error {
		published = append(published, got)
		if len(published) == 1 {
			return errors.New("broker unavailable")
		}
		return nil
	}, "relay-1", zap.NewNop())

	if worked, err := relay.RunOnce(context.Background()); !worked || err == nil || !repository.released || repository.published {
		t.Fatalf("failed publish: worked=%v err=%v released=%v published=%v", worked, err, repository.released, repository.published)
	}
	if worked, err := relay.RunOnce(context.Background()); !worked || err != nil || !repository.published {
		t.Fatalf("retried publish: worked=%v err=%v published=%v", worked, err, repository.published)
	}
	if len(published) != 2 || published[0] != request || published[1] != request {
		t.Fatalf("published requests = %+v", published)
	}
	if worked, err := relay.RunOnce(context.Background()); worked || err != nil {
		t.Fatalf("empty outbox: worked=%v err=%v", worked, err)
	}
}

const (
	storedTraceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
	storedSpanID      = "00f067aa0ba902b7"
	storedTraceparent = "00-" + storedTraceID + "-" + storedSpanID + "-01"
)

// recordSpans installs a global tracer provider that keeps ended spans in
// memory. Tests that call it must not run in parallel.
func recordSpans(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	return exporter
}

// publishOnce runs the relay once for request in ctx, and returns the only
// span that exporter recorded and the trace context that the publish
// function received.
func publishOnce(ctx context.Context, t *testing.T, exporter *tracetest.InMemoryExporter, request outbound.OutboxEvent,
	publishErr error, logger *zap.Logger,
) (tracetest.SpanStub, propagation.MapCarrier) {
	t.Helper()
	exporter.Reset()
	sent := propagation.MapCarrier{}
	relay := event.NewOutboxRelay(&relayRepository{request: request, pending: true}, func(ctx context.Context, _ outbound.OutboxEvent) error {
		propagation.TraceContext{}.Inject(ctx, sent)
		return publishErr
	}, "relay-1", logger)
	if worked, err := relay.RunOnce(ctx); !worked || (err != nil) != (publishErr != nil) {
		t.Fatalf("RunOnce = %v, %v", worked, err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	return spans[0], sent
}

func TestOutboxRelayContinuesTheStoredTraceContext(t *testing.T) {
	request := outbound.OutboxEvent{
		ID: "event-1", ChallengeID: "challenge-1", Purpose: "verify-email", Traceparent: storedTraceparent, Tracestate: "vendor=value",
	}
	span, sent := publishOnce(context.Background(), t, recordSpans(t), request, nil, zap.NewNop())

	if span.Name != "identity.outbox_publish" || span.SpanKind != trace.SpanKindProducer {
		t.Fatalf("span = %q, kind %v", span.Name, span.SpanKind)
	}
	if !span.Parent.IsRemote() || span.Parent.TraceID().String() != storedTraceID || span.Parent.SpanID().String() != storedSpanID ||
		span.SpanContext.TraceState().String() != "vendor=value" {
		t.Fatalf("parent = %+v, trace state %q", span.Parent, span.SpanContext.TraceState().String())
	}
	want := fmt.Sprintf("00-%s-%s-01", storedTraceID, span.SpanContext.SpanID())
	if sent.Get("traceparent") != want || sent.Get("tracestate") != "vendor=value" {
		t.Fatalf("publish context = %v, want traceparent %s", sent, want)
	}
}

func TestOutboxRelayStartsARootWithoutStoredTraceContext(t *testing.T) {
	// The caller span must not become the parent of an event without a
	// stored context.
	callerTrace, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	callerSpan, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	caller := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: callerTrace, SpanID: callerSpan, TraceFlags: trace.FlagsSampled,
	}))
	exporter := recordSpans(t)
	for _, traceparent := range []string{"", "not-a-traceparent", "00-00000000000000000000000000000000-" + storedSpanID + "-01"} {
		request := outbound.OutboxEvent{ID: "event-1", ChallengeID: "challenge-1", Purpose: "verify-email", Traceparent: traceparent}
		span, sent := publishOnce(caller, t, exporter, request, nil, zap.NewNop())

		if span.Parent.IsValid() || !span.SpanContext.IsValid() || span.SpanKind != trace.SpanKindProducer {
			t.Fatalf("traceparent %q: parent %v, kind %v", traceparent, span.Parent, span.SpanKind)
		}
		if !strings.Contains(sent.Get("traceparent"), span.SpanContext.TraceID().String()) || sent.Get("tracestate") != "" {
			t.Fatalf("traceparent %q: publish context = %v", traceparent, sent)
		}
	}
}

func TestOutboxRelayLogsAFailedPublishInItsSpan(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	request := outbound.OutboxEvent{
		ID: "event-1", ChallengeID: "challenge-1", Purpose: "verify-email", Traceparent: storedTraceparent,
	}
	span, _ := publishOnce(context.Background(), t, recordSpans(t), request, errors.New("broker refused event-1"), zap.New(core))

	if span.Status.Code != otelcodes.Error || len(span.Attributes) != 0 || len(span.Events) != 0 {
		t.Fatalf("failed span = %+v", span)
	}
	lines := logs.FilterMessage("outbox_publish_failed").All()
	if len(lines) != 1 || logs.Len() != 1 {
		t.Fatalf("relay logs = %v", logs.All())
	}
	fields := lines[0].ContextMap()
	if len(fields) != 2 || fields["trace_id"] != storedTraceID || fields["operation"] != "identity.outbox_publish" {
		t.Fatalf("outbox_publish_failed = %v", fields)
	}
}

func TestOutboxRelayLogsAFailedClaimWithoutATrace(t *testing.T) {
	exporter := recordSpans(t)
	core, logs := observer.New(zap.InfoLevel)
	relay := event.NewOutboxRelay(&relayRepository{claimErr: errors.New("database unavailable")},
		func(context.Context, outbound.OutboxEvent) error { return nil }, "relay-1", zap.New(core))

	if worked, err := relay.RunOnce(context.Background()); worked || err == nil {
		t.Fatalf("RunOnce = %v, %v", worked, err)
	}
	lines := logs.FilterMessage("outbox_publish_failed").All()
	if len(lines) != 1 || len(exporter.GetSpans()) != 0 {
		t.Fatalf("relay logs = %v, spans = %d", logs.All(), len(exporter.GetSpans()))
	}
	if fields := lines[0].ContextMap(); len(fields) != 1 || fields["operation"] != "identity.outbox_publish" {
		t.Fatalf("outbox_publish_failed = %v", fields)
	}
}
