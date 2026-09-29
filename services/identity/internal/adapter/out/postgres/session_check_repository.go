package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

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
