package inbound

import (
	"context"
)

type StartProviderLoginInput struct {
	Provider string
	Source   string
}

// CompleteProviderCallbackInput is the provider response to a callback route.
// Denied is true when the provider returned an error instead of a code.
type CompleteProviderCallbackInput struct {
	Provider string
	State    string
	Code     string
	Denied   bool
	Source   string
}

// ProviderCallbackResult is the one-time handoff code of a callback.
// Reauthentication is true when the attempt has the re-authentication purpose.
type ProviderCallbackResult struct {
	HandoffCode      string
	Reauthentication bool
}

type CreateProviderSessionInput struct {
	AttemptToken string
	HandoffCode  string
	Source       string
}

// CreateProviderSessionResult has the same fields as a password session.
type CreateProviderSessionResult = CreatePasswordSessionResult

type ProviderLoginService interface {
	StartProviderLogin(ctx context.Context, input StartProviderLoginInput) (StartProviderLoginResult, error)
	// CompleteProviderCallback makes sure that a provider callback is valid.
	// It returns a one-time handoff code and the purpose of the attempt.
	CompleteProviderCallback(ctx context.Context, input CompleteProviderCallbackInput) (ProviderCallbackResult, error)
	// CreateProviderSession claims a provider result with both handoff proofs
	// and creates one session for the linked account.
	CreateProviderSession(ctx context.Context, input CreateProviderSessionInput) (CreateProviderSessionResult, error)
}
