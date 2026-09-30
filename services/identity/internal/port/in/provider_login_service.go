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

type ProviderLoginService interface {
	StartProviderLogin(ctx context.Context, input StartProviderLoginInput) (StartProviderLoginResult, error)
}
