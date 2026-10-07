package metrics

import (
	"context"
	"errors"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
)

func TestNewProviderSetsServiceAndEnvironment(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := newProvider(reader, "identity-worker", "local")
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}

	attributes := map[string]string{}
	for _, attribute := range data.Resource.Attributes() {
		attributes[string(attribute.Key)] = attribute.Value.AsString()
	}
	if attributes["service.name"] != "identity-worker" || attributes["deployment.environment.name"] != "local" {
		t.Fatalf("resource attributes = %v", attributes)
	}
}

func TestNewProviderSetsSecondsHistogramBuckets(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := newProvider(reader, "identity-api", "local")
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	meter := provider.Meter("test")
	seconds, err := meter.Float64Histogram("test.duration", metric.WithUnit("s"))
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := meter.Float64Histogram("test.size", metric.WithUnit("By"))
	if err != nil {
		t.Fatal(err)
	}
	seconds.Record(context.Background(), 0.2)
	bytes.Record(context.Background(), 2048)

	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}

	want := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	if got := histogramBounds(t, data, "test.duration"); !slices.Equal(got, want) {
		t.Fatalf("seconds bounds = %v, want %v", got, want)
	}
	if got := histogramBounds(t, data, "test.size"); slices.Equal(got, want) {
		t.Fatalf("bytes histogram uses the seconds bounds %v", got)
	}
}

func TestNewReaderExportsEveryThirtySeconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		exporter := &recordingExporter{}
		provider := newProvider(newReader(exporter), "identity-api", "local")
		defer func() { _ = provider.Shutdown(context.Background()) }()
		counter, _ := provider.Meter("test").Int64Counter("test.count")
		counter.Add(context.Background(), 1)

		time.Sleep(30*time.Second - time.Millisecond)
		synctest.Wait()
		if got := exporter.count(); got != 0 {
			t.Fatalf("exports before 30s = %d, want 0", got)
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if got := exporter.count(); got != 1 {
			t.Fatalf("exports at 30s = %d, want 1", got)
		}
		time.Sleep(30*time.Second - time.Millisecond)
		synctest.Wait()
		if got := exporter.count(); got != 1 {
			t.Fatalf("exports before 60s = %d, want 1", got)
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if got := exporter.count(); got != 2 {
			t.Fatalf("exports at 60s = %d, want 2", got)
		}
	})
}

func TestNewReaderLimitsEachExportToTenSeconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		exporter := &recordingExporter{block: true}
		provider := newProvider(newReader(exporter), "identity-api", "local")
		counter, _ := provider.Meter("test").Int64Counter("test.count")
		counter.Add(context.Background(), 1)

		time.Sleep(30 * time.Second)
		synctest.Wait()
		if limit := exporter.limit(); limit != 10*time.Second {
			t.Fatalf("export limit = %s, want 10s", limit)
		}
		time.Sleep(10*time.Second - time.Millisecond)
		synctest.Wait()
		if got := exporter.failures(); got != 0 {
			t.Fatalf("failed exports before 10s = %d, want 0", got)
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if got := exporter.failures(); got != 1 {
			t.Fatalf("failed exports at 10s = %d, want 1", got)
		}
		flush(context.Background(), provider, time.Second)
	})
}

func TestRecordsDoNotWaitForABlockedExport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		exporter := &recordingExporter{block: true}
		provider := newProvider(newReader(exporter), "identity-api", "local")
		histogram, _ := provider.Meter("test").Float64Histogram("test.duration", metric.WithUnit("s"))
		histogram.Record(context.Background(), 0.01)
		time.Sleep(30 * time.Second)
		synctest.Wait()
		if exporter.count() != 1 || exporter.failures() != 0 {
			t.Fatalf("exports = %d, failures = %d, want one blocked export", exporter.count(), exporter.failures())
		}
		started := time.Now()

		for range 3000 {
			histogram.Record(context.Background(), 0.01)
		}

		// Time in the bubble moves only when every goroutine waits, so the
		// records finished while the export was still blocked.
		if elapsed := time.Since(started); elapsed != 0 || exporter.failures() != 0 {
			t.Fatalf("records took %s with %d finished exports", elapsed, exporter.failures())
		}
		flush(context.Background(), provider, time.Second)
	})
}

func TestNewReaderDropsAFailedIntervalAndExportsTheNext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		exporter := &recordingExporter{failFirst: true}
		provider := newProvider(newReader(exporter), "identity-api", "local")
		defer func() { _ = provider.Shutdown(context.Background()) }()
		counter, _ := provider.Meter("test").Int64Counter("test.count")

		counter.Add(context.Background(), 1)
		time.Sleep(30 * time.Second)
		synctest.Wait()
		counter.Add(context.Background(), 1)
		time.Sleep(30 * time.Second)
		synctest.Wait()

		if got := exporter.count(); got != 2 {
			t.Fatalf("export attempts = %d, want 2 without retries", got)
		}
		if got := exporter.lastSum(); got != 2 {
			t.Fatalf("exported sum after a failed interval = %d, want 2", got)
		}
	})
}

func TestFlushExportsTheLastIntervalAndStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		exporter := &recordingExporter{}
		provider := newProvider(newReader(exporter), "identity-api", "local")
		counter, _ := provider.Meter("test").Int64Counter("test.count")
		counter.Add(context.Background(), 3)

		flush(context.Background(), provider, time.Second)

		if exporter.count() != 1 || exporter.lastSum() != 3 {
			t.Fatalf("exports on shutdown = %d with sum %d, want 1 with sum 3", exporter.count(), exporter.lastSum())
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if got := exporter.count(); got != 1 {
			t.Fatalf("exports after shutdown = %d, want 1", got)
		}
	})
}

func TestFlushStopsAtTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		exporter := &recordingExporter{block: true}
		provider := newProvider(newReader(exporter), "identity-api", "local")
		counter, _ := provider.Meter("test").Int64Counter("test.count")
		counter.Add(context.Background(), 1)
		started := time.Now()

		flush(context.Background(), provider, 100*time.Millisecond)

		if exporter.count() != 1 || exporter.limit() != 100*time.Millisecond {
			t.Fatalf("exports = %d with limit %s, want 1 with limit 100ms", exporter.count(), exporter.limit())
		}
		if elapsed := time.Since(started); elapsed != 100*time.Millisecond {
			t.Fatalf("flush took %s with a 100ms limit", elapsed)
		}
	})
}

func TestStartExportsToTheEndpointOnStop(t *testing.T) {
	receiver := startReceiver(t, false)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://"+receiver.address)
	restoreGlobals(t)
	ctx, cancel := context.WithCancel(context.Background())

	stop := Start(ctx, "identity-worker", "local", func() {})
	counter, _ := otel.Meter("test").Int64Counter("test.count")
	counter.Add(context.Background(), 5)
	cancel()
	stop()

	request := receiver.lastRequest()
	if request == nil {
		t.Fatal("stop() exported no metrics")
	}
	resource := map[string]string{}
	var value int64
	for _, resourceMetrics := range request.GetResourceMetrics() {
		for _, attribute := range resourceMetrics.GetResource().GetAttributes() {
			resource[attribute.GetKey()] = attribute.GetValue().GetStringValue()
		}
		for _, scope := range resourceMetrics.GetScopeMetrics() {
			for _, metric := range scope.GetMetrics() {
				if metric.GetName() == "test.count" {
					value = metric.GetSum().GetDataPoints()[0].GetAsInt()
				}
			}
		}
	}
	if resource["service.name"] != "identity-worker" || resource["deployment.environment.name"] != "local" || value != 5 {
		t.Fatalf("exported resource = %v and value = %d", resource, value)
	}
}

func TestStopFlushesSpansAndMetricsAtTheSameTime(t *testing.T) {
	receiver := startReceiver(t, true)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://"+receiver.address)
	restoreGlobals(t)
	var spansStopped atomic.Bool
	stopSpans := func() {
		time.Sleep(4 * time.Second)
		spansStopped.Store(true)
	}

	stop := Start(context.Background(), "identity-api", "local", stopSpans)
	counter, _ := otel.Meter("test").Int64Counter("test.count")
	counter.Add(context.Background(), 1)
	started := time.Now()
	stop()
	elapsed := time.Since(started)

	if !spansStopped.Load() || receiver.lastRequest() == nil {
		t.Fatalf("spans stopped = %t, metric export attempted = %t", spansStopped.Load(), receiver.lastRequest() != nil)
	}
	// Each flush takes four seconds, so a stop that runs them one after
	// the other takes eight.
	if elapsed < 4*time.Second || elapsed > 5*time.Second {
		t.Fatalf("stop took %s, want about 4s", elapsed)
	}
}

func TestStartWithoutEndpointExportsNothing(t *testing.T) {
	// The SDK can send metrics to the metrics endpoint. Start must not
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
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "http://"+listener.Addr().String())
	restoreGlobals(t)

	stop := Start(context.Background(), "identity-api", "local", func() {})
	counter, _ := otel.Meter("test").Int64Counter("test.count")
	counter.Add(context.Background(), 1)
	stop()

	select {
	case <-connections:
		t.Fatal("Start() exported metrics without an endpoint")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestStartWithUnreachableEndpointDoesNotWait(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	restoreGlobals(t)
	started := time.Now()

	stop := Start(context.Background(), "identity-api", "local", func() {})
	histogram, _ := otel.Meter("test").Float64Histogram("test.duration", metric.WithUnit("s"))
	for range 3000 {
		histogram.Record(context.Background(), 0.01)
	}
	recorded := time.Since(started)
	stop()

	if recorded > time.Second {
		t.Fatalf("start and records took %s", recorded)
	}
	if elapsed := time.Since(started); elapsed > flushTimeout+time.Second {
		t.Fatalf("start, records, and stop took %s", elapsed)
	}
}

func TestStartInstallsTheMeterProvider(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	restoreGlobals(t)

	stop := Start(context.Background(), "workspace-api", "local", func() {})
	defer stop()

	if _, ok := otel.GetMeterProvider().(*sdkmetric.MeterProvider); !ok {
		t.Fatalf("meter provider = %T", otel.GetMeterProvider())
	}
}

// receiver is an OTLP metrics endpoint that keeps the last request.
type receiver struct {
	collectormetrics.UnimplementedMetricsServiceServer
	address string
	block   bool
	mu      sync.Mutex
	last    *collectormetrics.ExportMetricsServiceRequest
}

func startReceiver(t *testing.T, block bool) *receiver {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r := &receiver{address: listener.Addr().String(), block: block}
	server := grpc.NewServer()
	collectormetrics.RegisterMetricsServiceServer(server, r)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return r
}

// Export keeps the request. With block, it then waits until the client
// gives up.
func (r *receiver) Export(ctx context.Context, request *collectormetrics.ExportMetricsServiceRequest) (*collectormetrics.ExportMetricsServiceResponse, error) {
	r.mu.Lock()
	r.last = request
	r.mu.Unlock()
	if r.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &collectormetrics.ExportMetricsServiceResponse{}, nil
}

func (r *receiver) lastRequest() *collectormetrics.ExportMetricsServiceRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

func histogramBounds(t *testing.T, data metricdata.ResourceMetrics, name string) []float64 {
	t.Helper()
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				histogram, ok := metric.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Fatalf("metric %q has data %T, want a float64 histogram", name, metric.Data)
				}
				return histogram.DataPoints[0].Bounds
			}
		}
	}
	t.Fatalf("no metric %q", name)
	return nil
}

func restoreGlobals(t *testing.T) {
	t.Helper()
	provider := otel.GetMeterProvider()
	t.Cleanup(func() { otel.SetMeterProvider(provider) })
}

// recordingExporter records each export. With block, each export waits
// until its context ends. With failFirst, the first export fails.
type recordingExporter struct {
	block     bool
	failFirst bool
	mu        sync.Mutex
	exports   int
	failed    int
	timeout   time.Duration
	sum       int64
}

func (e *recordingExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return sdkmetric.DefaultTemporalitySelector(kind)
}

func (e *recordingExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

func (e *recordingExporter) Export(ctx context.Context, data *metricdata.ResourceMetrics) error {
	e.mu.Lock()
	e.exports++
	deadline, _ := ctx.Deadline()
	e.timeout = time.Until(deadline)
	failFirst := e.failFirst && e.exports == 1
	e.sum = 0
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if sum, ok := metric.Data.(metricdata.Sum[int64]); ok {
				e.sum += sum.DataPoints[0].Value
			}
		}
	}
	e.mu.Unlock()
	if e.block {
		<-ctx.Done()
		e.mu.Lock()
		e.failed++
		e.mu.Unlock()
		return ctx.Err()
	}
	if failFirst {
		return errors.New("export failed")
	}
	return nil
}

func (e *recordingExporter) ForceFlush(context.Context) error { return nil }

func (e *recordingExporter) Shutdown(context.Context) error { return nil }

func (e *recordingExporter) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.exports
}

func (e *recordingExporter) failures() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.failed
}

func (e *recordingExporter) limit() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.timeout
}

func (e *recordingExporter) lastSum() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sum
}
