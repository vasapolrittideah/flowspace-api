package postgrespool

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRegisterMetricsReportsConnectionsByState(t *testing.T) {
	metrics := registeredMetrics(t)

	count := metrics["db.client.connection.count"]
	gauge, ok := count.Data.(metricdata.Gauge[int64])
	if !ok || count.Unit != "{connection}" {
		t.Fatalf("connection count = %q %T", count.Unit, count.Data)
	}
	byState := map[string]int64{}
	for _, point := range gauge.DataPoints {
		state, _ := point.Attributes.Value(attribute.Key("db.client.connection.state"))
		if point.Attributes.Len() != 1 {
			t.Fatalf("connection count attributes = %v", point.Attributes.ToSlice())
		}
		byState[state.AsString()] = point.Value
	}
	if len(byState) != 2 || byState["idle"] != 3 || byState["used"] != 2 {
		t.Fatalf("connection count = %v, want idle 3 and used 2", byState)
	}
}

func TestRegisterMetricsReportsTheMaximumPoolSize(t *testing.T) {
	metrics := registeredMetrics(t)

	assertConnectionGauge(t, metrics["db.client.connection.max"], 10)
	if len(metrics) != 4 {
		t.Fatalf("metrics = %d, want 4", len(metrics))
	}
}

func TestRegisterMetricsReportsTheAcquireWaitInSeconds(t *testing.T) {
	wait := registeredMetrics(t)["flowspace.db.connection.acquire_wait"]

	sum, ok := wait.Data.(metricdata.Sum[float64])
	if !ok || wait.Unit != "s" || !sum.IsMonotonic || len(sum.DataPoints) != 1 ||
		sum.DataPoints[0].Value != 1.5 || sum.DataPoints[0].Attributes.Len() != 0 {
		t.Fatalf("acquire wait = %q %+v", wait.Unit, wait.Data)
	}
}

func TestRegisterMetricsReportsTheEmptyAcquires(t *testing.T) {
	empty := registeredMetrics(t)["flowspace.db.connection.empty_acquires"]

	sum, ok := empty.Data.(metricdata.Sum[int64])
	if !ok || empty.Unit != "{acquire}" || !sum.IsMonotonic || len(sum.DataPoints) != 1 ||
		sum.DataPoints[0].Value != 7 || sum.DataPoints[0].Attributes.Len() != 0 {
		t.Fatalf("empty acquires = %q %+v", empty.Unit, empty.Data)
	}
}

func TestRegisterMetricsReadsThePoolAtEachCollection(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	var current stats
	if _, err := registerMetrics(provider.Meter("test"), func() stats { return current }); err != nil {
		t.Fatal(err)
	}
	assertConnectionGauge(t, collect(t, reader)["db.client.connection.max"], 0)

	current = stats{idle: 1, used: 4, max: 5, acquireWait: 250 * time.Millisecond, emptyAcquires: 2}
	metrics := collect(t, reader)

	assertConnectionGauge(t, metrics["db.client.connection.max"], 5)
	count, ok := metrics["db.client.connection.count"].Data.(metricdata.Gauge[int64])
	if !ok || len(count.DataPoints) != 2 || count.DataPoints[0].Value+count.DataPoints[1].Value != 5 {
		t.Fatalf("connection count = %+v, want 1 idle and 4 used", metrics["db.client.connection.count"].Data)
	}
	wait, ok := metrics["flowspace.db.connection.acquire_wait"].Data.(metricdata.Sum[float64])
	if !ok || wait.DataPoints[0].Value != 0.25 {
		t.Fatalf("acquire wait = %+v, want 0.25", metrics["flowspace.db.connection.acquire_wait"].Data)
	}
	empty, ok := metrics["flowspace.db.connection.empty_acquires"].Data.(metricdata.Sum[int64])
	if !ok || empty.DataPoints[0].Value != 2 {
		t.Fatalf("empty acquires = %+v, want 2", metrics["flowspace.db.connection.empty_acquires"].Data)
	}
}

func TestRegisterMetricsReturnsEachMeterError(t *testing.T) {
	for _, fail := range []string{
		"db.client.connection.count", "db.client.connection.max", "flowspace.db.connection.acquire_wait",
		"flowspace.db.connection.empty_acquires", "callback",
	} {
		_, err := registerMetrics(failingMeter{fail: fail}, func() stats { return stats{} })
		if !errors.Is(err, errMeter) {
			t.Errorf("failing %s: error = %v, want errMeter", fail, err)
		}
	}
}

func TestNewPoolRunsWithoutMetricsWhenRegistrationFails(t *testing.T) {
	previousProvider, previousHandler := otel.GetMeterProvider(), otel.GetErrorHandler()
	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		otel.SetErrorHandler(previousHandler)
	})
	otel.SetMeterProvider(failingProvider{})
	var handled []error
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { handled = append(handled, err) }))
	pgx, err := pgxpool.New(context.Background(), "postgres://test@127.0.0.1:1/test")
	if err != nil {
		t.Fatal(err)
	}

	pool := newPool(pgx)
	pool.Close()

	if pool.Pool != pgx || pool.metrics != nil || len(handled) != 1 || !errors.Is(handled[0], errMeter) {
		t.Fatalf("pool = %+v, handled errors = %v", pool, handled)
	}
}

func TestPoolReportsItsStatisticsUntilItCloses(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(previous) })
	// pgxpool connects lazily, so this pool needs no database.
	pgx, err := pgxpool.New(context.Background(), "postgres://test@127.0.0.1:1/test?pool_max_conns=6")
	if err != nil {
		t.Fatal(err)
	}

	pool := newPool(pgx)
	assertConnectionGauge(t, collect(t, reader)["db.client.connection.max"], 6)
	pool.Close()

	if metrics := collect(t, reader); len(metrics) != 0 {
		t.Fatalf("closed pool reported %d metrics", len(metrics))
	}
}

// registeredMetrics collects the metrics of a pool with 3 idle and 2 used
// connections, 10 at most, a wait of 1.5 seconds, and 7 empty acquires.
func registeredMetrics(t *testing.T) map[string]metricdata.Metrics {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	read := func() stats {
		return stats{idle: 3, used: 2, max: 10, acquireWait: 1500 * time.Millisecond, emptyAcquires: 7}
	}
	if _, err := registerMetrics(provider.Meter("test"), read); err != nil {
		t.Fatal(err)
	}
	return collect(t, reader)
}

var errMeter = errors.New("meter failed")

// failingMeter fails to create the instrument named fail, or to register
// the callback when fail is "callback".
type failingMeter struct {
	noop.Meter
	fail string
}

func (m failingMeter) Int64ObservableGauge(name string, options ...metric.Int64ObservableGaugeOption) (metric.Int64ObservableGauge, error) {
	if name == m.fail {
		return nil, errMeter
	}
	return m.Meter.Int64ObservableGauge(name, options...)
}

func (m failingMeter) Float64ObservableCounter(name string, options ...metric.Float64ObservableCounterOption) (metric.Float64ObservableCounter, error) {
	if name == m.fail {
		return nil, errMeter
	}
	return m.Meter.Float64ObservableCounter(name, options...)
}

func (m failingMeter) Int64ObservableCounter(name string, options ...metric.Int64ObservableCounterOption) (metric.Int64ObservableCounter, error) {
	if name == m.fail {
		return nil, errMeter
	}
	return m.Meter.Int64ObservableCounter(name, options...)
}

func (m failingMeter) RegisterCallback(callback metric.Callback, instruments ...metric.Observable) (metric.Registration, error) {
	if m.fail == "callback" {
		return nil, errMeter
	}
	return m.Meter.RegisterCallback(callback, instruments...)
}

type failingProvider struct{ noop.MeterProvider }

func (failingProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return failingMeter{fail: "callback"}
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	metrics := map[string]metricdata.Metrics{}
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			metrics[metric.Name] = metric
		}
	}
	return metrics
}

func assertConnectionGauge(t *testing.T, metric metricdata.Metrics, want int64) {
	t.Helper()
	gauge, ok := metric.Data.(metricdata.Gauge[int64])
	if !ok || metric.Unit != "{connection}" || len(gauge.DataPoints) != 1 ||
		gauge.DataPoints[0].Value != want || gauge.DataPoints[0].Attributes.Len() != 0 {
		t.Fatalf("%s = %q %+v, want %d {connection}", metric.Name, metric.Unit, metric.Data, want)
	}
}
