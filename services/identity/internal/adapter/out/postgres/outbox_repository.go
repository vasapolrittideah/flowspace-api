package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrOutboxClaimLost = errors.New("outbox claim expired")

type OutboxRepository struct{ queries *sqlc.Queries }

var _ outbound.OutboxRepository = (*OutboxRepository)(nil)

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{queries: sqlc.New(pool)}
}

func (r *OutboxRepository) Claim(ctx context.Context, owner string) (outbound.OutboxEvent, bool, error) {
	row, err := r.queries.ClaimOutboxEvent(ctx, pgtype.Text{String: owner, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return outbound.OutboxEvent{}, false, nil
	}
	if err != nil {
		return outbound.OutboxEvent{}, false, err
	}
	return outbound.OutboxEvent{
		ID: uuid.UUID(row.ID.Bytes).String(), ChallengeID: uuid.UUID(row.ChallengeID.Bytes).String(), Purpose: row.Purpose,
	}, true, nil
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID, owner string) error {
	id, err := parseUUID(eventID)
	if err != nil {
		return err
	}
	changed, err := r.queries.MarkOutboxPublished(ctx, sqlc.MarkOutboxPublishedParams{
		ID: id, ClaimOwner: pgtype.Text{String: owner, Valid: true},
	})
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrOutboxClaimLost
	}
	return nil
}

func (r *OutboxRepository) Release(ctx context.Context, eventID, owner string, nextAttempt time.Time) error {
	id, err := parseUUID(eventID)
	if err != nil {
		return err
	}
	changed, err := r.queries.ReleaseOutboxClaim(ctx, sqlc.ReleaseOutboxClaimParams{
		ID: id, ClaimOwner: pgtype.Text{String: owner, Valid: true},
		NextAttemptAt: pgtype.Timestamptz{Time: nextAttempt, Valid: true},
	})
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrOutboxClaimLost
	}
	return nil
}
