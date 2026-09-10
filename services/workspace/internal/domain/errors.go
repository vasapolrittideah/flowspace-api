package domain

import (
	"errors"
	"fmt"
)

var (
	ErrCreateInProgress    = errors.New("workspace create in progress")
	ErrIdempotencyConflict = errors.New("idempotency key reused with another request")
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrNotFound            = errors.New("workspace not found")
	ErrUnauthenticated     = errors.New("unauthenticated")
)

type InvalidArgumentError struct {
	Field  string
	Reason string
}

func (e *InvalidArgumentError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func (e *InvalidArgumentError) Unwrap() error {
	return ErrInvalidArgument
}
