package inbound

import (
	"context"
	"time"
)

type StartProviderReauthenticationInput struct {
	Subject   string
	SessionID string
	Provider  string
	Source    string
}

type CompleteProviderReauthenticationInput struct {
	Subject      string
	SessionID    string
	AttemptToken string
	HandoffCode  string
	Source       string
}

type CompleteProviderReauthenticationResult struct{ ExpiresAt time.Time }

type ProviderReauthenticationService interface {
	// StartProviderReauthentication creates a provider attempt that only the
	// session of the input can complete.
	StartProviderReauthentication(ctx context.Context, input StartProviderReauthenticationInput) (StartProviderLoginResult, error)
	// CompleteProviderReauthentication claims the provider result of the
	// session and opens its re-authentication window.
	CompleteProviderReauthentication(ctx context.Context, input CompleteProviderReauthenticationInput) (CompleteProviderReauthenticationResult, error)
}
