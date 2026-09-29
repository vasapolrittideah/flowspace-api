package inbound

import "context"

type ResetPasswordInput struct {
	Email       string
	Code        string
	NewPassword string
	Source      string
}

type PasswordResetService interface {
	ResetPassword(ctx context.Context, input ResetPasswordInput) error
}
