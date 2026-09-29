package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres/sqlc"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func (r *AccountRepository) WithinPasswordResetTransaction(ctx context.Context, fn func(outbound.PasswordResetTransaction) error) error {
	return r.withinTransaction(ctx, func(tx *accountTransaction) error { return fn(tx) })
}

func (t *accountTransaction) GetCurrentPasswordResetChallenge(ctx context.Context, subject string) (outbound.ChallengeState, bool, error) {
	return t.getCurrentChallenge(ctx, subject, "password-reset")
}

func (t *accountTransaction) DeleteChallengeDelivery(ctx context.Context, challengeID string) error {
	id, err := parseUUID(challengeID)
	if err != nil {
		return err
	}
	_, err = t.queries.DeleteChallengeDelivery(ctx, id)
	return err
}

func (t *accountTransaction) UpdatePasswordHash(ctx context.Context, subject, hash string) error {
	changed, err := t.queries.UpdatePasswordHash(ctx, sqlc.UpdatePasswordHashParams{Subject: subject, PasswordHash: hash})
	if err == nil && changed != 1 {
		return errors.New("password account unavailable")
	}
	return err
}

func (t *accountTransaction) RevokePasswordSessions(ctx context.Context, subject string) error {
	_, err := t.queries.RevokeAccountSessions(ctx, subject)
	return err
}

func (t *accountTransaction) QueuePasswordChangeNotice(ctx context.Context, subject string) error {
	_, err := t.queries.QueuePasswordChangeNotice(ctx, subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("password notice account unavailable")
	}
	return err
}
