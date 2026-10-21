package postgrespool

import "errors"

var (
	ErrInvalidConfiguration = errors.New("invalid database configuration")
	ErrUnavailable          = errors.New("database unavailable")
)
