package inbound

import (
	"context"
	"time"
)

type ClaimAccountInput struct {
	Email       string
	Code        string
	NewPassword string
	Source      string
}

type ClaimAccountResult struct {
	Subject              string
	AccessToken          string
	RefreshToken         string
	AccessTokenExpiresAt time.Time
}

type AccountClaimService interface {
	ClaimUnverifiedAccount(ctx context.Context, input ClaimAccountInput) (ClaimAccountResult, error)
}
