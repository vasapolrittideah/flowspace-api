package inbound

import (
	"context"
	"time"
)

type ReauthenticateWithPasswordInput struct {
	Subject   string
	SessionID string
	Password  string
	Source    string
}

type ReauthenticateWithPasswordResult struct{ ExpiresAt time.Time }

type PasswordReauthenticationService interface {
	ReauthenticateWithPassword(ctx context.Context, input ReauthenticateWithPasswordInput) (ReauthenticateWithPasswordResult, error)
}
