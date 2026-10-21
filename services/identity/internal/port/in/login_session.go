package inbound

import "time"

type CreatePasswordSessionResult struct {
	Subject               string
	EmailVerified         bool
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
	SessionExpiresAt      time.Time
}
