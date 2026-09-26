package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type SessionRepository struct{ queries *sqlc.Queries }

var _ outbound.SessionCheckRepository = (*SessionRepository)(nil)

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

func (r *SessionRepository) Check(ctx context.Context, subject, sessionID string) (bool, error) {
	id, err := parseUUID(sessionID)
	if err != nil {
		return false, outbound.ErrUnauthenticated
	}
	verifiedAt, err := r.queries.GetActiveSessionState(ctx, sqlc.GetActiveSessionStateParams{Subject: subject, SessionID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, outbound.ErrUnauthenticated
	}
	return verifiedAt.Valid, err
}
