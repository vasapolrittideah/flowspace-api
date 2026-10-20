package outbound

import (
	"context"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
)

type PasswordReauthenticationTransaction interface {
	ReauthenticationTransaction
	GetActiveAccountForSession(ctx context.Context, subject, sessionID string) (AccountState, error)
	LockPasswordAccount(ctx context.Context, subject string) (PasswordAccount, error)
	ReauthenticateSession(ctx context.Context, subject, sessionID string) error
}

type PasswordReauthenticationRepository interface {
	WithinPasswordReauthenticationTransaction(ctx context.Context, fn func(PasswordReauthenticationTransaction) error) (domain.ReauthenticationWindow, error)
}
