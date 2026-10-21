package app

import (
	"errors"

	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var (
	ErrInvalidClaimCode                = errors.New("invalid claim code")
	ErrClaimUnavailable                = errors.New("account claim unavailable")
	ErrAllLogoutUnavailable            = errors.New("all-session logout unavailable")
	ErrClaimCodeUnavailable            = errors.New("claim code unavailable")
	ErrInvalidReauthenticationCode     = errors.New("invalid re-authentication code")
	ErrReauthenticationEmailUnverified = errors.New("verified email required")
	ErrCurrentLogoutUnavailable        = errors.New("current session logout unavailable")
	ErrInvalidLimitKey                 = errors.New("invalid limit key")
	ErrLimitUnavailable                = errors.New("limit repository unavailable")
	ErrRateLimited                     = errors.New("rate limit exceeded")
	ErrInvalidCredentials              = errors.New("invalid credentials")
	ErrLoginUnavailable                = errors.New("password login unavailable")
	ErrPasswordResetCodeUnavailable    = errors.New("password recovery request unavailable")
	ErrInvalidPasswordResetCode        = errors.New("invalid password reset code")
	ErrPasswordResetUnavailable        = errors.New("password reset unavailable")
	ErrProviderLoginUnavailable        = errors.New("provider login unavailable")
	ErrInvalidProviderCallback         = errors.New("invalid provider callback")
	// ErrProviderHandoffRejected covers an unknown, pending, expired,
	// exhausted, failed, or claimed attempt alike.
	ErrProviderHandoffRejected     = errors.New("provider handoff rejected")
	ErrProviderAccountUnavailable  = errors.New("provider account unavailable")
	ErrReauthenticationRequired    = outbound.ErrReauthenticationRequired
	ErrReauthenticationUnavailable = errors.New("re-authentication unavailable")
	ErrSessionCheckUnavailable     = errors.New("session check unavailable")
	ErrInvalidRefreshToken         = errors.New("invalid refresh token")
	ErrUnauthenticatedRefresh      = errors.New("refresh token not authenticated")
	ErrRefreshUnavailable          = errors.New("refresh unavailable")
	ErrSessionIssue                = errors.New("session issuance failed")
	ErrSignupUnavailable           = errors.New("signup unavailable")
	ErrEmailAlreadyVerified        = errors.New("email already verified")
	ErrVerificationCodeUnavailable = errors.New("verification code unavailable")
	ErrInvalidVerificationCode     = errors.New("invalid verification code")
)
