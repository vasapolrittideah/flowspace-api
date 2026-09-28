// Package postgrespool opens PostgreSQL pools during service startup.
package postgrespool

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidConfiguration = errors.New("invalid database configuration")
	ErrUnavailable          = errors.New("database unavailable")
)

// Open returns a connected pool or a credential-safe startup error.
func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
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
	return pool, nil
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
