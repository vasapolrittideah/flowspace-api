package outbound

import "context"

type SignupTransaction interface {
	SessionRepository
	VerificationCodeDeliveryTransaction
	CreateAccount(ctx context.Context, subject, email, passwordHash string) error
}

type SignupRepository interface {
	WithinTransaction(ctx context.Context, fn func(SignupTransaction) error) error
}
