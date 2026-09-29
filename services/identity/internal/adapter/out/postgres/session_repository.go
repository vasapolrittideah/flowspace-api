package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type SessionRepository struct{ queries *sqlc.Queries }

var (
	_ outbound.SessionCheckRepository         = (*SessionRepository)(nil)
	_ outbound.CurrentSessionLogoutRepository = (*SessionRepository)(nil)
)

func NewSessionRepository(db sqlc.DBTX) *SessionRepository {
	return &SessionRepository{queries: sqlc.New(db)}
}

func (r *SessionRepository) Create(ctx context.Context, subject string, hash []byte) (outbound.SessionRecord, error) {
	row, err := r.queries.CreateSession(ctx, sqlc.CreateSessionParams{
		AccountSubject: subject, RefreshTokenHash: hash,
	})
	if err != nil {
		return outbound.SessionRecord{}, err
	}
	return outbound.SessionRecord{
		ID: uuid.UUID(row.ID.Bytes).String(), CreatedAt: row.CreatedAt.Time,
		IdleExpiresAt: row.IdleExpiresAt.Time, AbsoluteExpiresAt: row.AbsoluteExpiresAt.Time,
	}, nil
}
