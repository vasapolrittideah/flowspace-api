package outbound

import (
	"context"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
)

type ProviderReauthenticationTransaction interface {
	ReauthenticationTransaction
	// CreateProviderAttempt stores a new attempt and returns its expiry.
	CreateProviderAttempt(ctx context.Context, attempt ProviderAttempt) (time.Time, error)
	// ClaimProviderReauthenticationResult claims the result of a live
	// re-authentication attempt of the session once when both proofs match.
	// It reports false for any other attempt.
	ClaimProviderReauthenticationResult(ctx context.Context, subject, sessionID string, attemptVerifier, handoffVerifier [32]byte) (ProviderResult, bool, error)
	// HasProviderLink reports whether the provider identity is linked to the subject.
	HasProviderLink(ctx context.Context, subject, provider, providerSubject string) (bool, error)
	ReauthenticateSession(ctx context.Context, subject, sessionID string) error
}

type ProviderReauthenticationRepository interface {
	WithinProviderReauthenticationTransaction(ctx context.Context, fn func(ProviderReauthenticationTransaction) error) (domain.ReauthenticationWindow, error)
}
