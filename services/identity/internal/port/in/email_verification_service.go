package inbound

import (
	"context"
)

type RequestEmailVerificationCodeInput struct {
	Subject   string
	SessionID string
	Source    string
}

type EmailVerificationCodeService interface {
	RequestEmailVerificationCode(ctx context.Context, input RequestEmailVerificationCodeInput) error
}

type VerifyEmailInput struct {
	Subject   string
	SessionID string
	Source    string
	Code      string
}

type EmailVerificationService interface {
	VerifyEmail(ctx context.Context, input VerifyEmailInput) error
}
