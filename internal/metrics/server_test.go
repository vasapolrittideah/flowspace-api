package metrics

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc/codes"
)

func TestServerDurationsRecordBoundedHTTPValues(t *testing.T) {
	for _, test := range []struct {
		name, method, route string
		status              int
		want                attribute.Set
	}{
		{"matched route", "POST", "/v1/accounts", 201, attribute.NewSet(
			attribute.String("http.request.method", "POST"), attribute.String("http.route", "/v1/accounts"),
			attribute.Int("http.response.status_code", 201),
		)},
		{"no matched route", "GET", "", 404, attribute.NewSet(
			attribute.String("http.request.method", "GET"), attribute.String("http.route", "unmatched"),
			attribute.Int("http.response.status_code", 404),
		)},
		{"client-defined method", "SECRET-METHOD", "/v1/accounts", 501, attribute.NewSet(
			attribute.String("http.request.method", "_OTHER"), attribute.String("http.route", "/v1/accounts"),
			attribute.Int("http.response.status_code", 501),
		)},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			provider := newProvider(reader, "identity-api", "local")
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

			NewServerDurations(provider.Meter("test")).RecordHTTP(context.Background(), 250*time.Millisecond, test.method, test.route, test.status)

			point := onlyDurationPoint(t, reader, "http.server.request.duration")
			if !point.Attributes.Equals(&test.want) {
				t.Fatalf("attributes = %v, want %v", point.Attributes.ToSlice(), test.want.ToSlice())
			}
			if point.Count != 1 || point.Sum != 0.25 {
				t.Fatalf("count = %d, sum = %v, want 1 and 0.25", point.Count, point.Sum)
			}
		})
	}
}

func TestServerDurationsRecordBoundedRPCValues(t *testing.T) {
	for _, test := range []struct {
		name, method string
		code         codes.Code
		want         attribute.Set
	}{
		{"registered method", "/flowspace.identity.v1.IdentityService/CreateAccount", codes.InvalidArgument, attribute.NewSet(
			attribute.String("rpc.method", "flowspace.identity.v1.IdentityService/CreateAccount"), attribute.Int("rpc.grpc.status_code", 3),
		)},
		{"unregistered method", "", codes.Unimplemented, attribute.NewSet(
			attribute.String("rpc.method", "unknown"), attribute.Int("rpc.grpc.status_code", 12),
		)},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			provider := newProvider(reader, "workspace-api", "local")
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

			NewServerDurations(provider.Meter("test")).RecordRPC(context.Background(), 10*time.Millisecond, test.method, test.code)

			point := onlyDurationPoint(t, reader, "rpc.server.call.duration")
			if !point.Attributes.Equals(&test.want) {
				t.Fatalf("attributes = %v, want %v", point.Attributes.ToSlice(), test.want.ToSlice())
			}
			if point.Count != 1 || point.Sum != 0.01 {
				t.Fatalf("count = %d, sum = %v, want 1 and 0.01", point.Count, point.Sum)
			}
			if want := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}; !slices.Equal(point.Bounds, want) {
				t.Fatalf("bounds = %v, want %v", point.Bounds, want)
			}
		})
	}
}

func TestNewServerDurationsReportsInstrumentErrors(t *testing.T) {
	for name, failing := range map[string][]string{
		"HTTP":     {"http.server.request.duration"},
		"RPC":      {"rpc.server.call.duration"},
		"both":     {"http.server.request.duration", "rpc.server.call.duration"},
		"no error": nil,
	} {
		t.Run(name, func(t *testing.T) {
			previous := otel.GetErrorHandler()
			t.Cleanup(func() { otel.SetErrorHandler(previous) })
			var reported []error
			otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { reported = append(reported, err) }))

			durations := NewServerDurations(failingMeter{failing: failing})
			durations.RecordHTTP(context.Background(), time.Millisecond, "GET", "", 200)
			durations.RecordRPC(context.Background(), time.Millisecond, "", codes.OK)

			if len(failing) == 0 {
				if len(reported) != 0 {
					t.Fatalf("reported errors = %v, want none", reported)
				}
				return
			}
			if len(reported) != 1 {
				t.Fatalf("reported errors = %v, want 1 joined error", reported)
			}
			for _, instrument := range failing {
				if !errors.Is(reported[0], instrumentError(instrument)) {
					t.Fatalf("reported error %v does not name %s", reported[0], instrument)
				}
			}
		})
	}
}

// instrumentError is the error that failingMeter returns for name.
type instrumentError string

func (e instrumentError) Error() string { return string(e) + " unavailable" }

// failingMeter returns a usable no-op histogram and an error for each name in
// failing.
type failingMeter struct {
	noop.Meter
	failing []string
}

func (m failingMeter) Float64Histogram(name string, _ ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	if slices.Contains(m.failing, name) {
		return noop.Float64Histogram{}, instrumentError(name)
	}
	return noop.Float64Histogram{}, nil
}

// onlyDurationPoint returns the one data point of the seconds histogram name.
func onlyDurationPoint(t *testing.T, reader sdkmetric.Reader, name string) metricdata.HistogramDataPoint[float64] {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok || metric.Unit != "s" || len(histogram.DataPoints) != 1 {
				t.Fatalf("metric %q = %+v, want 1 seconds histogram point", name, metric)
			}
			return histogram.DataPoints[0]
		}
	}
	t.Fatalf("no metric %q", name)
	return metricdata.HistogramDataPoint[float64]{}
}
