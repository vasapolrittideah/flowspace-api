package outbound

import (
	"context"
	"errors"
)

var ErrAccountExists = errors.New("account already exists")

type SignupTransaction interface {
	SessionRepository
	VerificationCodeDeliveryTransaction
	CreateAccount(ctx context.Context, subject, email, passwordHash string) error
}

type SignupRepository interface {
	WithinTransaction(ctx context.Context, fn func(SignupTransaction) error) error
}
