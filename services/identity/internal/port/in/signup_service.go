package inbound

import (
	"context"
	"time"
)

type CreateAccountInput struct {
	Email    string
	Password string
	Source   string
}

type CreateAccountResult struct {
	Subject              string
	AccessToken          string
	RefreshToken         string
	AccessTokenExpiresAt time.Time
}

type SignupService interface {
	CreateAccount(ctx context.Context, input CreateAccountInput) (CreateAccountResult, error)
}
