package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type LimitRepository struct{ pool *pgxpool.Pool }

var _ outbound.LimitRepository = (*LimitRepository)(nil)

func NewLimitRepository(pool *pgxpool.Pool) *LimitRepository { return &LimitRepository{pool: pool} }

func (r *LimitRepository) Record(ctx context.Context, scope, key, action string, maximum, dailyMaximum int, window, interval time.Duration) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, scope+":"+action+":"+key)
	if err != nil {
		return false, err
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT statement_timestamp()`).Scan(&now); err != nil {
		return false, err
	}
	queries := sqlc.New(tx)
	allowed, err := recordLimit(ctx, queries, scope, key, action, maximum, dailyMaximum, now, window, interval)
	if err != nil || !allowed {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *LimitRepository) RecordLogin(ctx context.Context, source, emailKey string, sourceMaximum, emailMaximum int, sourceWindow, emailWindow time.Duration) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, key := range []string{"source:password-login:" + source, "email:password-login:" + emailKey} {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
			return false, err
		}
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT statement_timestamp()`).Scan(&now); err != nil {
		return false, err
	}
	queries := sqlc.New(tx)
	allowed, err := recordLimit(ctx, queries, "source", source, "password-login", sourceMaximum, 0, now, sourceWindow, 0)
	if err != nil || !allowed {
		return false, err
	}
	allowed, err = recordLimit(ctx, queries, "email", emailKey, "password-login", emailMaximum, 0, now, emailWindow, 0)
	if err != nil || !allowed {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func recordLimit(ctx context.Context, queries *sqlc.Queries, scope, key, action string, maximum, dailyMaximum int, now time.Time, window, interval time.Duration) (bool, error) {
	usage, err := queries.GetLimitUsage(ctx, sqlc.GetLimitUsageParams{
		Scope: scope, CounterKey: key, Action: action,
		HourCutoff: pgtype.Timestamptz{Time: now.Add(-window), Valid: true},
		DayCutoff:  pgtype.Timestamptz{Time: now.Add(-24 * time.Hour), Valid: true},
	})
	if err != nil {
		return false, err
	}
	if usage.HourlyCount >= int64(maximum) || dailyMaximum > 0 && usage.DailyCount >= int64(dailyMaximum) || interval > 0 && usage.LatestAt.Valid && now.Sub(usage.LatestAt.Time) < interval {
		return false, nil
	}
	_, err = queries.IncrementLimitCounter(ctx, sqlc.IncrementLimitCounterParams{
		Scope: scope, CounterKey: key, Action: action,
		WindowStart: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return false, err
	}
	return true, nil
}
