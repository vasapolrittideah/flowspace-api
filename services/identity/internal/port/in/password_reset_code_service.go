package inbound

import "context"

type RequestPasswordResetCodeInput struct {
	Email  string
	Source string
}

type PasswordResetCodeService interface {
	RequestPasswordResetCode(ctx context.Context, input RequestPasswordResetCodeInput) error
}
