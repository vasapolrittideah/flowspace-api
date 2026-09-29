package postgres

import (
	"context"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func (r *SessionRepository) RevokeCurrent(ctx context.Context, subject, sessionID string) error {
	id, err := parseUUID(sessionID)
	if err != nil {
		return outbound.ErrUnauthenticated
	}
	rows, err := r.queries.RevokeCurrentSession(ctx, sqlc.RevokeCurrentSessionParams{SessionID: id, Subject: subject})
	if err != nil {
		return err
	}
	if rows != 1 {
		return outbound.ErrUnauthenticated
	}
	return nil
}
