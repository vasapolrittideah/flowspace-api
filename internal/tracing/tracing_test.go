package tracing

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewProviderSetsServiceAndEnvironment(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := newProvider(exporter, "identity-api", "local")
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	_, span := provider.Tracer("test").Start(context.Background(), "test")
	span.End()
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported %d spans, want 1", len(spans))
	}
	attributes := map[string]string{}
	for _, attribute := range spans[0].Resource.Attributes() {
		attributes[string(attribute.Key)] = attribute.Value.AsString()
	}
	if attributes["service.name"] != "identity-api" || attributes["deployment.environment.name"] != "local" {
		t.Fatalf("resource attributes = %v", attributes)
	}
}

func TestNewProviderFollowsSamplerEnvironment(t *testing.T) {
	tests := []struct {
		name            string
		ratio           string
		unsampledParent bool
		wantSampled     bool
	}{
		{name: "root with ratio 1.0", ratio: "1.0", wantSampled: true},
		{name: "child of unsampled parent", ratio: "1.0", unsampledParent: true, wantSampled: false},
		{name: "root with ratio 0.0", ratio: "0.0", wantSampled: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("OTEL_TRACES_SAMPLER", "parentbased_traceidratio")
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", test.ratio)
			provider := newProvider(nil, "identity-api", "local")
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

			parent := context.Background()
			if test.unsampledParent {
				parent = trace.ContextWithRemoteSpanContext(parent, trace.NewSpanContext(trace.SpanContextConfig{
					TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}, Remote: true,
				}))
			}
			_, span := provider.Tracer("test").Start(parent, "test")
			defer span.End()

			if sampled := span.SpanContext().IsSampled(); sampled != test.wantSampled {
				t.Fatalf("sampled = %t, want %t", sampled, test.wantSampled)
			}
		})
	}
}

func TestStartWithoutEndpointExportsNothing(t *testing.T) {
	// The SDK would send spans to the traces endpoint. Start must not
	// export them, because OTEL_EXPORTER_OTLP_ENDPOINT is not set.
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	connections := make(chan struct{}, 1)
	go func() {
		if connection, err := listener.Accept(); err == nil {
			connections <- struct{}{}
			_ = connection.Close()
		}
	}()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://"+listener.Addr().String())
	restoreGlobals(t)

	stop := Start(context.Background(), zap.NewNop(), "identity-api", "local")
	_, span := otel.Tracer("test").Start(context.Background(), "test")
	span.End()
	stop()

	select {
	case <-connections:
		t.Fatal("Start() exported spans without an endpoint")
	case <-time.After(100 * time.Millisecond):
	}
	if !span.SpanContext().IsValid() {
		t.Fatal("span has no trace context")
	}
}

func TestStartWithUnreachableEndpointDoesNotWait(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	restoreGlobals(t)
	started := time.Now()

	stop := Start(context.Background(), zap.NewNop(), "identity-api", "local")
	for range 3000 {
		ctx, span := otel.Tracer("test").Start(context.Background(), "test")
		if !span.SpanContext().IsValid() || !trace.SpanContextFromContext(ctx).IsSampled() {
			t.Fatal("span has no sampled trace context")
		}
		span.End()
	}
	stop()

	if elapsed := time.Since(started); elapsed > flushTimeout+time.Second {
		t.Fatalf("start, spans, and stop took %s", elapsed)
	}
}

func TestTracingCreatesPropagatedContext(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	restoreGlobals(t)

	stop := Start(context.Background(), zap.NewNop(), "workspace-api", "local")
	defer stop()
	ctx, span := otel.Tracer("test").Start(context.Background(), "test")
	defer span.End()
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	if carrier.Get("traceparent") == "" || !span.SpanContext().HasTraceID() {
		t.Fatal("trace context was not created and propagated")
	}
}

func TestStartLogsTracingErrors(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	restoreGlobals(t)
	core, logs := observer.New(zap.WarnLevel)

	stop := Start(context.Background(), zap.New(core), "identity-worker", "local")
	defer stop()
	otel.Handle(errors.New("export failed"))

	entries := logs.FilterMessage("tracing_error").All()
	if len(entries) != 1 || entries[0].ContextMap()["error"] != "export failed" {
		t.Fatalf("logs = %v", logs.All())
	}
}

func TestNewProviderLimitsEachExport(t *testing.T) {
	exporter := &blockingExporter{release: make(chan struct{})}
	close(exporter.release)
	provider := newProvider(exporter, "identity-api", "local")
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	_, span := provider.Tracer("test").Start(context.Background(), "test")
	span.End()
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}

	limit := exporter.deadline.Sub(exporter.started)
	if limit <= exportTimeout-time.Second || limit > exportTimeout {
		t.Fatalf("export limit = %s, want %s", limit, exportTimeout)
	}
}

func TestNewProviderDropsSpansWhenQueueIsFull(t *testing.T) {
	exporter := &blockingExporter{release: make(chan struct{})}
	provider := newProvider(exporter, "identity-api", "local")
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	started := time.Now()

	for range 5000 {
		_, span := provider.Tracer("test").Start(context.Background(), "test")
		span.End()
	}
	elapsed := time.Since(started)
	close(exporter.release)
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}

	if elapsed > time.Second {
		t.Fatalf("ending spans waited %s for a blocked export", elapsed)
	}
	// The queue holds maxQueueSize spans, and one blocked batch holds at most 512 more.
	if exported := exporter.count(); exported > maxQueueSize+512 {
		t.Fatalf("exported %d spans, want at most %d", exported, maxQueueSize+512)
	}
}

func TestFlushStopsAtTimeout(t *testing.T) {
	exporter := &blockingExporter{release: make(chan struct{})}
	t.Cleanup(func() { close(exporter.release) })
	provider := newProvider(exporter, "identity-api", "local")
	_, span := provider.Tracer("test").Start(context.Background(), "test")
	span.End()
	started := time.Now()

	flush(context.Background(), provider, 100*time.Millisecond)

	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("flush took %s with a 100ms limit", elapsed)
	}
}

func restoreGlobals(t *testing.T) {
	t.Helper()
	provider, propagator, handler := otel.GetTracerProvider(), otel.GetTextMapPropagator(), otel.GetErrorHandler()
	t.Cleanup(func() {
		otel.SetTracerProvider(provider)
		otel.SetTextMapPropagator(propagator)
		otel.SetErrorHandler(handler)
	})
}

// blockingExporter waits for release before it accepts each batch.
type blockingExporter struct {
	release  chan struct{}
	mu       sync.Mutex
	spans    int
	started  time.Time
	deadline time.Time
}

func (e *blockingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	e.started = time.Now()
	e.deadline, _ = ctx.Deadline()
	e.mu.Unlock()
	select {
	case <-e.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans += len(spans)
	return nil
}

func (e *blockingExporter) Shutdown(context.Context) error { return nil }

func (e *blockingExporter) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.spans
}
