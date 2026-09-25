package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
)

type SessionRepository struct{ queries *sqlc.Queries }

func NewSessionRepository(tx pgx.Tx) *SessionRepository {
	return &SessionRepository{queries: sqlc.New(tx)}
}

func (r *SessionRepository) Create(ctx context.Context, subject string, hash []byte) (app.SessionRecord, error) {
	row, err := r.queries.CreateSession(ctx, sqlc.CreateSessionParams{
		AccountSubject: subject, RefreshTokenHash: hash,
	})
	if err != nil {
		return app.SessionRecord{}, err
	}
	return app.SessionRecord{
		ID: uuid.UUID(row.ID.Bytes).String(), CreatedAt: row.CreatedAt.Time,
		IdleExpiresAt: row.IdleExpiresAt.Time, AbsoluteExpiresAt: row.AbsoluteExpiresAt.Time,
	}, nil
}
