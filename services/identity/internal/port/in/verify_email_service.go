package inbound

import "context"

type VerifyEmailInput struct {
	Subject   string
	SessionID string
	Source    string
	Code      string
}

type VerifyEmailService interface {
	VerifyEmail(ctx context.Context, input VerifyEmailInput) error
}
