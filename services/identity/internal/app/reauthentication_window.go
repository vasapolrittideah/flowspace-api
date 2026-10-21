package app

import (
	"context"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type reauthenticationWindowOpener interface {
	outbound.ReauthenticationTransaction
	ReauthenticateSession(ctx context.Context, subject, sessionID string) error
}

// openWindow opens a closed window of the session at commit and keeps an open
// one. It returns the session deadline that the commit must meet.
func openWindow(ctx context.Context, tx reauthenticationWindowOpener, subject, sessionID string) (time.Time, error) {
	window, now, err := tx.LockSessionForReauthentication(ctx, subject, sessionID)
	if err != nil {
		return time.Time{}, err
	}
	if !window.IsOpen(now) {
		if err := tx.ReauthenticateSession(ctx, subject, sessionID); err != nil {
			return time.Time{}, err
		}
	}
	deadline := sessionReauthenticationDeadline(window)
	tx.RequireCommitBefore(deadline)
	return deadline, nil
}

func sessionReauthenticationDeadline(window domain.ReauthenticationWindow) time.Time {
	if window.IdleExpiresAt.Before(window.AbsoluteExpiresAt) {
		return window.IdleExpiresAt
	}
	return window.AbsoluteExpiresAt
}
