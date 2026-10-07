// Package metrics sets up bounded, non-blocking metric export for Flowspace
// processes.
package metrics

import (
	"context"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/vasapolrittideah/flowspace-api/internal/tracing"
)

const (
	// exportInterval and exportTimeout are the limits that ADR-0038 sets.
	exportInterval = 30 * time.Second
	exportTimeout  = 10 * time.Second
	// flushTimeout matches the span flush. The two flushes run at the same
	// time, so that both fit inside the termination grace period of each pod
	// after the five-second server shutdown.
	flushTimeout = 4 * time.Second
)

// durationBounds are the latency buckets of the specification in seconds.
// They cover the five-second request cap of ADR-0010.
var durationBounds = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// Start installs the global meter provider with the resource of the spans.
// The SDK reads the endpoint from the standard OTEL_* variables. Without
// OTEL_EXPORTER_OTLP_ENDPOINT, instruments record nothing and the provider
// exports nothing. The returned function runs stopSpans, the stop function of
// tracing.Start, while it exports the last interval for at most four
// seconds and stops the provider. Start uses the error handler that
// tracing.Start installs.
func Start(ctx context.Context, service, environment string, stopSpans func()) func() {
	var reader sdkmetric.Reader
	if exporter := newExporter(ctx); exporter != nil {
		reader = newReader(exporter)
	}
	provider := newProvider(reader, service, environment)
	otel.SetMeterProvider(provider)
	return func() {
		var flushes sync.WaitGroup
		flushes.Go(stopSpans)
		flushes.Go(func() { flush(context.WithoutCancel(ctx), provider, flushTimeout) })
		flushes.Wait()
	}
}

// newExporter returns nil when no endpoint is set or the exporter cannot
// start, because telemetry never stops a process. The gRPC connection is
// lazy, so an unreachable endpoint does not delay startup.
func newExporter(ctx context.Context) sdkmetric.Exporter {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return nil
	}
	exporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		otel.Handle(err)
		return nil
	}
	return exporter
}

// newReader exports in the background, so a request never waits for an
// export. A failed export drops its interval, and the reader does not retry
// it after the export limit.
func newReader(exporter sdkmetric.Exporter) sdkmetric.Reader {
	return sdkmetric.NewPeriodicReader(exporter,
		sdkmetric.WithInterval(exportInterval),
		sdkmetric.WithTimeout(exportTimeout),
	)
}

func newProvider(reader sdkmetric.Reader, service, environment string) *sdkmetric.MeterProvider {
	options := []sdkmetric.Option{
		sdkmetric.WithResource(tracing.Resource(service, environment)),
		sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Kind: sdkmetric.InstrumentKindHistogram, Unit: "s"},
			sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: durationBounds}},
		)),
	}
	if reader != nil {
		options = append(options, sdkmetric.WithReader(reader))
	}
	return sdkmetric.NewMeterProvider(options...)
}

// flush drops the values that it cannot export before the timeout.
func flush(ctx context.Context, provider *sdkmetric.MeterProvider, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_ = provider.Shutdown(ctx)
}
