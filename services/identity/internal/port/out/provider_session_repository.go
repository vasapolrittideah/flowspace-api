package outbound

import "context"

// ProviderResult is the verified provider identity of a claimed attempt.
type ProviderResult struct {
	Provider string
	Subject  string
}

// LinkedAccount is the active account linked to a provider identity.
type LinkedAccount struct {
	Subject       string
	EmailVerified bool
}

type ProviderSessionTransaction interface {
	SessionRepository
	// ClaimProviderResult claims the result of a live attempt once when both
	// proofs match. It reports false for any other attempt state.
	ClaimProviderResult(ctx context.Context, attemptVerifier, handoffVerifier [32]byte) (ProviderResult, bool, error)
	// LockLinkedAccount locks the active account linked to the provider
	// identity. It reports false when no such account exists.
	LockLinkedAccount(ctx context.Context, provider, providerSubject string) (LinkedAccount, bool, error)
}

type ProviderSessionRepository interface {
	WithinProviderSessionTransaction(ctx context.Context, fn func(ProviderSessionTransaction) error) error
}
