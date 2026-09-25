package inbound

import "context"

type RequestEmailVerificationCodeInput struct {
	Subject   string
	SessionID string
	Source    string
}

type VerificationCodeService interface {
	RequestEmailVerificationCode(ctx context.Context, input RequestEmailVerificationCodeInput) error
}
