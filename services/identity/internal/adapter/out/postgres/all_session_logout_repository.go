package postgres

import (
	"context"
)

func (r *AccountRepository) RevokeAll(ctx context.Context, subject, sessionID string) error {
	return r.withinTransaction(ctx, func(tx *accountTransaction) error {
		if _, err := tx.GetActiveAccountForSession(ctx, subject, sessionID); err != nil {
			return err
		}
		_, err := tx.queries.RevokeAccountSessions(ctx, subject)
		return err
	})
}
