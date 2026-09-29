package inbound

import "context"

type PasswordRecoveryRequestInput struct {
	Email  string
	Source string
}

type PasswordRecoveryRequestService interface {
	RequestPasswordResetCode(ctx context.Context, input PasswordRecoveryRequestInput) error
}
