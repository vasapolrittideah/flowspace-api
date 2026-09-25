package inbound

import "context"

type RequestEmailVerificationCodeInput struct {
	Subject   string
	SessionID string
	Source    string
}

type VerifyEmailInput struct {
	Subject   string
	SessionID string
	Source    string
	Code      string
}

type VerificationCodeService interface {
	RequestEmailVerificationCode(ctx context.Context, input RequestEmailVerificationCodeInput) error
	VerifyEmail(ctx context.Context, input VerifyEmailInput) error
}
