package postgres

import (
	"bytes"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type SessionRefreshRepository struct{ pool *pgxpool.Pool }

var _ outbound.SessionRefreshRepository = (*SessionRefreshRepository)(nil)

func NewSessionRefreshRepository(pool *pgxpool.Pool) *SessionRefreshRepository {
	return &SessionRefreshRepository{pool: pool}
}

func (r *SessionRefreshRepository) Rotate(ctx context.Context, oldHash, newHash []byte, issue func(outbound.RefreshSessionRecord) error) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := sqlc.New(tx)
	id, err := queries.FindRefreshSession(ctx, oldHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, outbound.ErrUnauthenticated
	}
	if err != nil {
		return false, err
	}
	if err := lockRefreshAccount(ctx, queries, id); err != nil {
		return false, err
	}
	session, err := queries.GetRefreshSessionForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, outbound.ErrUnauthenticated
	}
	if err != nil {
		return false, err
	}
	if !session.Active.Valid || !session.Active.Bool {
		return false, outbound.ErrUnauthenticated
	}
	if !bytes.Equal(session.RefreshTokenHash, oldHash) {
		if err := queries.RevokeRefreshSession(ctx, id); err != nil {
			return false, err
		}
		return true, tx.Commit(ctx)
	}
	if err := queries.RecordRotatedRefreshToken(ctx, sqlc.RecordRotatedRefreshTokenParams{TokenHash: oldHash, SessionID: id}); err != nil {
		return false, err
	}
	rotated, err := queries.RotateCurrentRefreshToken(ctx, sqlc.RotateCurrentRefreshTokenParams{
		NewHash: newHash, SessionID: id, OldHash: oldHash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, outbound.ErrUnauthenticated
	}
	if err != nil {
		return false, err
	}
	if err := issue(outbound.RefreshSessionRecord{
		Subject: session.AccountSubject, ID: uuid.UUID(id.Bytes).String(), IssuedAt: rotated.IssuedAt.Time,
		IdleExpiresAt: rotated.IdleExpiresAt.Time, AbsoluteExpiresAt: rotated.AbsoluteExpiresAt.Time,
	}); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

func lockRefreshAccount(ctx context.Context, queries *sqlc.Queries, id pgtype.UUID) error {
	subject, err := queries.GetRefreshSessionSubject(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	if _, err := queries.GetActiveAccountForUpdate(ctx, subject); errors.Is(err, pgx.ErrNoRows) {
		return outbound.ErrUnauthenticated
	} else if err != nil {
		return err
	}
	return nil
}
