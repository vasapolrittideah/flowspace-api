package outbound

import "errors"

var (
	ErrUnauthenticated          = errors.New("unauthenticated")
	ErrReauthenticationRequired = errors.New("re-authentication required")
	ErrAccountExists            = errors.New("account already exists")
)
