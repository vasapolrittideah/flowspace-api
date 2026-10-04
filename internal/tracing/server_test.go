package tracing

import (
	"context"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
)

func endedSpan(t *testing.T, record func(trace.Span)) sdktrace.ReadOnlySpan {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	_, span := provider.Tracer("test").Start(t.Context(), "test")
	record(span)
	span.End()
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported %d spans, want 1", len(spans))
	}
	return spans[0].Snapshot()
}

func attributeValue(span sdktrace.ReadOnlySpan, key string) (attribute.Value, bool) {
	for _, item := range span.Attributes() {
		if string(item.Key) == key {
			return item.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestHTTPMethodKeepsStandardMethodsOnly(t *testing.T) {
	for method, want := range map[string]string{
		http.MethodGet: "GET", http.MethodPost: "POST", http.MethodDelete: "DELETE", http.MethodOptions: "OPTIONS",
		"get": "_OTHER", "PURGE": "_OTHER", "": "_OTHER",
	} {
		if got := HTTPMethod(method); got != want {
			t.Errorf("HTTPMethod(%q) = %q, want %q", method, got, want)
		}
	}
}

func TestSetHTTPStatusMarksOnlyServerErrors(t *testing.T) {
	for status, wantError := range map[int]bool{200: false, 302: false, 400: false, 404: false, 499: false, 500: true, 503: true, 599: true} {
		span := endedSpan(t, func(span trace.Span) { SetHTTPStatus(span, status) })
		if value, ok := attributeValue(span, "http.response.status_code"); !ok || value.AsInt64() != int64(status) {
			t.Errorf("status %d: attribute = %v", status, value.String())
		}
		if got := span.Status().Code == otelcodes.Error; got != wantError {
			t.Errorf("status %d: error status = %v, want %v", status, got, wantError)
		}
	}
}

func TestSetGRPCStatusMarksOnlyServerFailures(t *testing.T) {
	for code, wantError := range map[codes.Code]bool{
		codes.OK: false, codes.InvalidArgument: false, codes.Unauthenticated: false, codes.NotFound: false, codes.Unimplemented: false,
		codes.Internal: true, codes.Unavailable: true, codes.DeadlineExceeded: true, codes.Unknown: true,
	} {
		span := endedSpan(t, func(span trace.Span) { SetGRPCStatus(span, code) })
		if value, ok := attributeValue(span, "rpc.grpc.status_code"); !ok || value.AsInt64() != int64(code) {
			t.Errorf("code %v: attribute = %v", code, value.String())
		}
		if got := span.Status().Code == otelcodes.Error; got != wantError {
			t.Errorf("code %v: error status = %v, want %v", code, got, wantError)
		}
	}
}

func TestGRPCStatusReadsTheStatusTrailer(t *testing.T) {
	for value, want := range map[string]codes.Code{"0": codes.OK, "13": codes.Internal, "14": codes.Unavailable} {
		header := http.Header{}
		header.Set("Grpc-Status", value)
		if got, ok := GRPCStatus(header); !ok || got != want {
			t.Errorf("GRPCStatus(%q) = %v, %v", value, got, ok)
		}
	}
	for _, value := range []string{"", "x", "-1", "17", "4294967296"} {
		header := http.Header{}
		if value != "" {
			header.Set("Grpc-Status", value)
		}
		if got, ok := GRPCStatus(header); ok {
			t.Errorf("GRPCStatus(%q) = %v, want no status", value, got)
		}
	}
}
