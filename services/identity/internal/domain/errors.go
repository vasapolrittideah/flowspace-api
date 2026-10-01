package domain

import "errors"

var (
	ErrCodeGenerationUnavailable  = errors.New("code generation unavailable")
	ErrInvalidAttemptToken        = errors.New("invalid attempt token")
	ErrInvalidChallenge           = errors.New("invalid challenge")
	ErrInvalidHandoffCode         = errors.New("invalid handoff code")
	ErrInvalidEmail               = errors.New("invalid email address")
	ErrInvalidPassword            = errors.New("invalid password")
	ErrInvalidProvider            = errors.New("invalid provider")
	ErrInvalidProviderProof       = errors.New("invalid provider proof")
	ErrProviderUnavailable        = errors.New("provider unavailable")
	ErrPasswordCheckUnavailable   = errors.New("password check unavailable")
	ErrPasswordHashingUnavailable = errors.New("password hashing unavailable")
)
