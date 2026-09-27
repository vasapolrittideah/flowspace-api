package inbound

import (
	"context"
	"time"
)

type CreatePasswordSessionInput struct {
	Email    string
	Password string
	Source   string
}

type CreatePasswordSessionResult struct {
	Subject               string
	EmailVerified         bool
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
	SessionExpiresAt      time.Time
}

type PasswordLoginService interface {
	CreatePasswordSession(ctx context.Context, input CreatePasswordSessionInput) (CreatePasswordSessionResult, error)
}
