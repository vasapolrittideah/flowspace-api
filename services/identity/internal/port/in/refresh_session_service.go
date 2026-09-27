package inbound

import (
	"context"
	"time"
)

type RefreshSessionResult struct {
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
	SessionExpiresAt      time.Time
}

type RefreshSessionService interface {
	RefreshSession(ctx context.Context, token string) (RefreshSessionResult, error)
}
