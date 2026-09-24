package domain

import "errors"

var (
	ErrCodeGenerationUnavailable  = errors.New("code generation unavailable")
	ErrInvalidChallenge           = errors.New("invalid challenge")
	ErrInvalidEmail               = errors.New("invalid email address")
	ErrInvalidPassword            = errors.New("invalid password")
	ErrPasswordCheckUnavailable   = errors.New("password check unavailable")
	ErrPasswordHashingUnavailable = errors.New("password hashing unavailable")
)
