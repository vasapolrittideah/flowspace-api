package inbound

import "time"

// StartProviderLoginResult is a new provider attempt of provider login or
// provider re-authentication.
type StartProviderLoginResult struct {
	AuthorizationURL string
	AttemptToken     string
	ExpiresAt        time.Time
}
