// Package postgrespool opens PostgreSQL pools during service startup.
package postgrespool

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	ErrInvalidConfiguration = errors.New("invalid database configuration")
	ErrUnavailable          = errors.New("database unavailable")
)

// Pool is a pgx pool that reports its statistics through the global meter
// provider until it closes.
type Pool struct {
	*pgxpool.Pool
	metrics metric.Registration
}

// newPool starts the pool metrics. A metrics error never stops a process,
// so the pool then runs without metrics.
func newPool(pool *pgxpool.Pool) *Pool {
	registration, err := registerMetrics(otel.Meter("github.com/vasapolrittideah/flowspace-api/internal/postgrespool"),
		func() stats { return readStats(pool) })
	if err != nil {
		otel.Handle(err)
	}
	return &Pool{Pool: pool, metrics: registration}
}

// Close stops the pool metrics and closes the pool.
func (p *Pool) Close() {
	if p.metrics != nil {
		_ = p.metrics.Unregister()
	}
	p.Pool.Close()
}

// Open returns a connected pool or a credential-safe startup error.
func Open(ctx context.Context, databaseURL string) (*Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, ErrInvalidConfiguration
	}
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := retry(startupCtx, pool.Ping); err != nil {
		pool.Close()
		return nil, ErrUnavailable
	}
	return newPool(pool), nil
}

func retry(ctx context.Context, ping func(context.Context) error) error {
	for {
		if ping(ctx) == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}
