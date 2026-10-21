package outbound

import "context"

type PasswordSessionTransaction interface {
	SessionRepository
	LockPasswordAccount(ctx context.Context, subject string) (PasswordAccount, error)
}

type PasswordLoginRepository interface {
	FindPasswordAccount(ctx context.Context, email string) (PasswordAccount, bool, error)
	WithinPasswordSessionTransaction(ctx context.Context, fn func(PasswordSessionTransaction) error) error
}
