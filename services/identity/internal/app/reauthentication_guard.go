package app

import (
	"context"
	"errors"

	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

// RequireReauthentication guards the caller's transaction at admission and before commit.
func RequireReauthentication(ctx context.Context, tx outbound.ReauthenticationTransaction, subject, sessionID string) error {
	if subject == "" || sessionID == "" {
		return outbound.ErrUnauthenticated
	}
	if tx == nil {
		return ErrReauthenticationUnavailable
	}
	window, now, err := tx.LockSessionForReauthentication(ctx, subject, sessionID)
	if err != nil {
		if errors.Is(err, outbound.ErrUnauthenticated) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return ErrReauthenticationUnavailable
	}
	if !window.IsOpen(now) {
		return ErrReauthenticationRequired
	}
	tx.RequireCommitBefore(window.ExpiresAt())
	return nil
}
