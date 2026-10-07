package postgrespool

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// stats holds the pool values that the metrics report.
type stats struct {
	idle, used, max int32
	acquireWait     time.Duration
	emptyAcquires   int64
}

func readStats(pool *pgxpool.Pool) stats {
	stat := pool.Stat()
	return stats{
		idle: stat.IdleConns(), used: stat.AcquiredConns(), max: stat.MaxConns(),
		acquireWait: stat.AcquireDuration(), emptyAcquires: stat.EmptyAcquireCount(),
	}
}

// registerMetrics reports the pool values that read returns at each export.
func registerMetrics(meter metric.Meter, read func() stats) (metric.Registration, error) {
	count, err := meter.Int64ObservableGauge("db.client.connection.count", metric.WithUnit("{connection}"),
		metric.WithDescription("Connections in the database pool"))
	if err != nil {
		return nil, err
	}
	maxConns, err := meter.Int64ObservableGauge("db.client.connection.max", metric.WithUnit("{connection}"),
		metric.WithDescription("Maximum size of the database pool"))
	if err != nil {
		return nil, err
	}
	acquireWait, err := meter.Float64ObservableCounter("flowspace.db.connection.acquire_wait", metric.WithUnit("s"),
		metric.WithDescription("Total time that callers waited for a connection"))
	if err != nil {
		return nil, err
	}
	emptyAcquires, err := meter.Int64ObservableCounter("flowspace.db.connection.empty_acquires", metric.WithUnit("{acquire}"),
		metric.WithDescription("Acquires that waited because no idle connection existed"))
	if err != nil {
		return nil, err
	}
	idle := metric.WithAttributes(semconv.DBClientConnectionStateIdle)
	used := metric.WithAttributes(semconv.DBClientConnectionStateUsed)
	return meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		values := read()
		observer.ObserveInt64(count, int64(values.idle), idle)
		observer.ObserveInt64(count, int64(values.used), used)
		observer.ObserveInt64(maxConns, int64(values.max))
		observer.ObserveFloat64(acquireWait, values.acquireWait.Seconds())
		observer.ObserveInt64(emptyAcquires, values.emptyAcquires)
		return nil
	}, count, maxConns, acquireWait, emptyAcquires)
}
