package inbound

import (
	"context"
	"time"
)

type StartProviderLoginInput struct {
	Provider string
	Source   string
}

type StartProviderLoginResult struct {
	AuthorizationURL string
	AttemptToken     string
	ExpiresAt        time.Time
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

type CreateProviderSessionInput struct {
	AttemptToken string
	HandoffCode  string
	Source       string
}

// CreateProviderSessionResult has the same fields as a password session.
type CreateProviderSessionResult = CreatePasswordSessionResult

type ProviderLoginService interface {
	StartProviderLogin(ctx context.Context, input StartProviderLoginInput) (StartProviderLoginResult, error)
	// CompleteProviderCallback validates a provider callback and returns a
	// one-time handoff code.
	CompleteProviderCallback(ctx context.Context, input CompleteProviderCallbackInput) (string, error)
	// CreateProviderSession claims a provider result with both handoff proofs
	// and creates one session for the linked account.
	CreateProviderSession(ctx context.Context, input CreateProviderSessionInput) (CreateProviderSessionResult, error)
}
