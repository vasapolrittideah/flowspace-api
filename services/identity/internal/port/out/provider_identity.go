package outbound

import "context"

// ProviderCodeExchange holds the attempt values that bind a provider
// authorization code to one login attempt.
type ProviderCodeExchange struct {
	Code         string
	CodeVerifier string
	Nonce        string
	CallbackURL  string
}

// ProviderIdentity is the identity evidence that a provider proved.
type ProviderIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	HostedDomain  string
}

// ProviderIdentityVerifier exchanges an authorization code and verifies the
// provider identity. It returns domain.ErrInvalidProviderProof for rejected
// proof and domain.ErrProviderUnavailable for a failed provider call.
type ProviderIdentityVerifier interface {
	VerifyProviderIdentity(ctx context.Context, exchange ProviderCodeExchange) (ProviderIdentity, error)
}
