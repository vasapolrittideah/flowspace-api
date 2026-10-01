package outbound

import "context"

// ProviderResult is the verified provider identity of a claimed attempt.
type ProviderResult struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	HostedDomain  string
}

// LinkedAccount is the active account linked to a provider identity.
type LinkedAccount struct {
	Subject       string
	EmailVerified bool
}

type ProviderSessionTransaction interface {
	SessionRepository
	VerificationCodeDeliveryTransaction
	// ClaimProviderResult claims the result of a live attempt once when both
	// proofs match. It reports false for any other attempt state.
	ClaimProviderResult(ctx context.Context, attemptVerifier, handoffVerifier [32]byte) (ProviderResult, bool, error)
	// LockLinkedAccount locks the active account linked to the provider
	// identity. It reports false when no such account exists.
	LockLinkedAccount(ctx context.Context, provider, providerSubject string) (LinkedAccount, bool, error)
	// CreateProviderAccount creates a passwordless account and its provider
	// link. It reports false when an active account owns the email.
	CreateProviderAccount(ctx context.Context, account NewProviderAccount) (bool, error)
}

// NewProviderAccount is a provider-only account and its provider link.
type NewProviderAccount struct {
	Subject         string
	Email           string
	EmailVerified   bool
	Provider        string
	ProviderSubject string
}

type ProviderSessionRepository interface {
	WithinProviderSessionTransaction(ctx context.Context, fn func(ProviderSessionTransaction) error) error
}
