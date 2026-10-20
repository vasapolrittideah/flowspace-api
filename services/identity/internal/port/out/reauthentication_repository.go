package outbound

import (
	"context"
	"errors"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
)

var ErrReauthenticationRequired = errors.New("re-authentication required")

// ReauthenticationTransaction holds the session locks until the caller commits.
type ReauthenticationTransaction interface {
	LockSessionForReauthentication(ctx context.Context, subject, sessionID string) (domain.ReauthenticationWindow, time.Time, error)
	// RequireCommitBefore enforces the earliest deadline during database commit.
	RequireCommitBefore(deadline time.Time)
}
