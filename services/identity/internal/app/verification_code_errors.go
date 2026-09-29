package app

import "errors"

var (
	ErrEmailAlreadyVerified        = errors.New("email already verified")
	ErrVerificationCodeUnavailable = errors.New("verification code unavailable")
	ErrInvalidVerificationCode     = errors.New("invalid verification code")
)
