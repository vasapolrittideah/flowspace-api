package app

import (
	"context"
	"errors"
	"time"

	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var (
	ErrInvalidLimitKey  = errors.New("invalid limit key")
	ErrLimitUnavailable = errors.New("limit repository unavailable")
	ErrRateLimited      = errors.New("rate limit exceeded")
)

type LimitService struct{ repository outbound.LimitRepository }

func NewLimitService(repository outbound.LimitRepository) *LimitService {
	return &LimitService{repository: repository}
}

func (l *LimitService) Signup(ctx context.Context, source string) error {
	return l.record(ctx, "source", source, "signup", 10, 0, 0)
}

func (l *LimitService) CodeRequest(ctx context.Context, source string) error {
	return l.record(ctx, "source", source, "code-request", 60, 0, 0)
}

func (l *LimitService) WrongCode(ctx context.Context, source string) error {
	return l.record(ctx, "source", source, "code-guess", 100, 0, 0)
}

func (l *LimitService) CodeIssue(ctx context.Context, account string) error {
	return l.record(ctx, "account", account, "code-request", 5, 0, time.Minute)
}

func (l *LimitService) AccountWrongCode(ctx context.Context, account string) error {
	return l.record(ctx, "account", account, "code-guess", 10, 20, 0)
}

func (l *LimitService) record(ctx context.Context, scope, key, action string, maximum, dailyMaximum int, interval time.Duration) error {
	if key == "" {
		return ErrInvalidLimitKey
	}
	allowed, err := l.repository.Record(ctx, scope, key, action, maximum, dailyMaximum, time.Hour, interval)
	if err != nil {
		return errors.Join(ErrLimitUnavailable, err)
	}
	if !allowed {
		return ErrRateLimited
	}
	return nil
}
