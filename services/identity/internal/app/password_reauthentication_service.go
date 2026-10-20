package app

import (
	"context"
	"errors"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type PasswordReauthenticationService struct {
	repository outbound.PasswordReauthenticationRepository
	limit      func(context.Context, string, string) error
	hashSlots  chan struct{}
	verify     func(context.Context, string, string) (bool, error)
}

var _ inbound.PasswordReauthenticationService = (*PasswordReauthenticationService)(nil)

func NewPasswordReauthenticationService(repository outbound.PasswordReauthenticationRepository, limit func(context.Context, string, string) error) *PasswordReauthenticationService {
	return &PasswordReauthenticationService{repository: repository, limit: limit, hashSlots: make(chan struct{}, 2), verify: domain.VerifyPassword}
}

func (s *PasswordReauthenticationService) ReauthenticateWithPassword(ctx context.Context, input inbound.ReauthenticateWithPasswordInput) (inbound.ReauthenticateWithPasswordResult, error) {
	if s.repository == nil || s.limit == nil {
		return inbound.ReauthenticateWithPasswordResult{}, ErrReauthenticationUnavailable
	}
	if input.Subject == "" || input.SessionID == "" {
		return inbound.ReauthenticateWithPasswordResult{}, outbound.ErrUnauthenticated
	}
	password, err := domain.NormalizeLoginPassword(input.Password)
	if err != nil {
		return inbound.ReauthenticateWithPasswordResult{}, err
	}
	window, err := s.repository.WithinPasswordReauthenticationTransaction(ctx, func(tx outbound.PasswordReauthenticationTransaction) error {
		if _, _, err := tx.LockSessionForReauthentication(ctx, input.Subject, input.SessionID); err != nil {
			return err
		}
		account, err := tx.GetActiveAccountForSession(ctx, input.Subject, input.SessionID)
		if err != nil {
			return err
		}
		if err := s.limit(ctx, input.Source, account.Email); err != nil {
			return err
		}
		if err := s.checkPassword(ctx, tx, input.Subject, password); err != nil {
			return err
		}
		window, now, err := tx.LockSessionForReauthentication(ctx, input.Subject, input.SessionID)
		if err != nil {
			return err
		}
		if !window.IsOpen(now) {
			if err := tx.ReauthenticateSession(ctx, input.Subject, input.SessionID); err != nil {
				return err
			}
		}
		end := window.IdleExpiresAt
		if window.AbsoluteExpiresAt.Before(end) {
			end = window.AbsoluteExpiresAt
		}
		tx.RequireCommitBefore(end)
		return nil
	})
	if err != nil {
		return inbound.ReauthenticateWithPasswordResult{}, passwordReauthenticationError(err)
	}
	return inbound.ReauthenticateWithPasswordResult{ExpiresAt: window.ExpiresAt()}, nil
}

func (s *PasswordReauthenticationService) checkPassword(ctx context.Context, tx outbound.PasswordReauthenticationTransaction, subject, password string) error {
	credentials, err := tx.LockPasswordAccount(ctx, subject)
	if err != nil && !errors.Is(err, outbound.ErrUnauthenticated) {
		return err
	}
	hasPassword := err == nil
	hash := unknownAccountVerifier
	if hasPassword {
		hash = credentials.PasswordHash
	}
	select {
	case s.hashSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	matched, err := s.verify(ctx, password, hash)
	<-s.hashSlots
	if err != nil {
		return err
	}
	if !hasPassword || !matched {
		return ErrInvalidCredentials
	}
	return nil
}

func passwordReauthenticationError(err error) error {
	switch {
	case errors.Is(err, outbound.ErrReauthenticationRequired):
		err = outbound.ErrUnauthenticated
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, outbound.ErrUnauthenticated), errors.Is(err, ErrRateLimited), errors.Is(err, ErrLimitUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
	default:
		err = ErrReauthenticationUnavailable
	}
	return err
}
