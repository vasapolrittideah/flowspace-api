//go:build integration

package postgrespool

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	postgrescontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestOpenReportsThePoolStatistics(t *testing.T) {
	ctx := context.Background()
	container, err := postgrescontainer.Run(ctx, "postgres:18-alpine",
		postgrescontainer.WithDatabase("metrics"),
		postgrescontainer.WithUsername("metrics"),
		postgrescontainer.WithPassword("metrics"),
		postgrescontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable", "pool_max_conns=1")
	if err != nil {
		t.Fatal(err)
	}
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(previous) })
	pool, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// The ping of Open waited while the pool built its first connection, so
	// the pool has one empty acquire with a wait. This acquire holds that
	// connection.
	acquireCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := pool.Acquire(acquireCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	metrics := collect(t, reader)
	stat := pool.Stat()

	if stat.EmptyAcquireCount() < 1 || stat.AcquireDuration() <= 0 || stat.AcquiredConns() != 1 {
		t.Fatalf("pool statistics are not as planned: %d empty acquires, %s wait, %d used",
			stat.EmptyAcquireCount(), stat.AcquireDuration(), stat.AcquiredConns())
	}
	assertMatchesStat(t, metrics, stat)
}

// assertMatchesStat compares each reported value with the pool statistics.
func assertMatchesStat(t *testing.T, metrics map[string]metricdata.Metrics, stat *pgxpool.Stat) {
	t.Helper()
	count, ok := metrics["db.client.connection.count"].Data.(metricdata.Gauge[int64])
	if !ok {
		t.Fatalf("connection count = %T", metrics["db.client.connection.count"].Data)
	}
	byState := map[string]int64{}
	for _, point := range count.DataPoints {
		state, _ := point.Attributes.Value(attribute.Key("db.client.connection.state"))
		byState[state.AsString()] = point.Value
	}
	if byState["idle"] != int64(stat.IdleConns()) || byState["used"] != int64(stat.AcquiredConns()) {
		t.Fatalf("connection count = %v, pool has %d idle and %d used", byState, stat.IdleConns(), stat.AcquiredConns())
	}
	assertConnectionGauge(t, metrics["db.client.connection.max"], int64(stat.MaxConns()))
	wait, ok := metrics["flowspace.db.connection.acquire_wait"].Data.(metricdata.Sum[float64])
	if !ok || wait.DataPoints[0].Value != stat.AcquireDuration().Seconds() {
		t.Fatalf("acquire wait = %+v, pool has %s", metrics["flowspace.db.connection.acquire_wait"].Data, stat.AcquireDuration())
	}
	empty, ok := metrics["flowspace.db.connection.empty_acquires"].Data.(metricdata.Sum[int64])
	if !ok || empty.DataPoints[0].Value != stat.EmptyAcquireCount() {
		t.Fatalf("empty acquires = %+v, pool has %d", metrics["flowspace.db.connection.empty_acquires"].Data, stat.EmptyAcquireCount())
	}
}
