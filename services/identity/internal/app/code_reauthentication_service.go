package app

import (
	"context"
	"errors"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type CodeReauthenticationService struct {
	repository                                        outbound.CodeReauthenticationRepository
	protector                                         outbound.DeliveryProtector
	requestLimit, guessSourceLimit, guessAccountLimit func(context.Context, string) error
	verifierKey                                       []byte
}

var _ inbound.CodeReauthenticationService = (*CodeReauthenticationService)(nil)

func NewCodeReauthenticationService(repository outbound.CodeReauthenticationRepository, protector outbound.DeliveryProtector, requestLimit, guessSourceLimit, guessAccountLimit func(context.Context, string) error, key []byte) *CodeReauthenticationService {
	return &CodeReauthenticationService{repository: repository, protector: protector, requestLimit: requestLimit, guessSourceLimit: guessSourceLimit, guessAccountLimit: guessAccountLimit, verifierKey: key}
}

func (s *CodeReauthenticationService) RequestReauthenticationCode(ctx context.Context, input inbound.RequestReauthenticationCodeInput) error {
	if input.Subject == "" || input.SessionID == "" {
		return outbound.ErrUnauthenticated
	}
	if s.repository == nil || s.protector == nil || s.requestLimit == nil || len(s.verifierKey) != 32 {
		return ErrReauthenticationUnavailable
	}
	err := s.repository.WithinReauthenticationCodeIssueTransaction(ctx, func(tx outbound.ReauthenticationCodeIssueTransaction) error { return s.issue(ctx, tx, input) })
	return codeReauthenticationError(err)
}

func (s *CodeReauthenticationService) issue(ctx context.Context, tx outbound.ReauthenticationCodeIssueTransaction, input inbound.RequestReauthenticationCodeInput) error {
	window, now, err := tx.LockSessionForReauthentication(ctx, input.Subject, input.SessionID)
	if err != nil {
		return err
	}
	account, err := tx.GetActiveAccountForSession(ctx, input.Subject, input.SessionID)
	if err != nil {
		return err
	}
	if err := s.requestLimit(ctx, input.Source); err != nil {
		return err
	}
	if !account.EmailVerified {
		return ErrReauthenticationEmailUnverified
	}
	last, err := tx.GetLatestReauthenticationCodeIssue(ctx, input.Subject)
	if err != nil {
		return err
	}
	if !last.IsZero() && now.Before(last.Add(time.Minute)) {
		return nil
	}
	allowed, err := tx.CanIssueCode(ctx, input.Subject)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}
	tx.RequireCommitBefore(sessionReauthenticationDeadline(window))
	return s.queue(ctx, tx, input.Subject, account.Email, now)
}

func (s *CodeReauthenticationService) queue(ctx context.Context, tx outbound.ReauthenticationCodeIssueTransaction, subject, email string, now time.Time) error {
	code, verifier, _, err := domain.NewChallenge(s.verifierKey, subject, email, domain.PurposeReauthenticate, now)
	if err != nil {
		return err
	}
	if err := tx.ReplaceReauthenticationChallenge(ctx, subject); err != nil {
		return err
	}
	id, err := tx.CreateReauthenticationChallenge(ctx, subject, email, verifier)
	if err != nil {
		return err
	}
	material, err := s.protector.Protect(id, string(domain.PurposeReauthenticate), subject, email, code)
	if err != nil {
		return err
	}
	if err := tx.StoreDelivery(ctx, id, material); err != nil {
		return err
	}
	return tx.CreateOutboxEvent(ctx, id)
}

func (s *CodeReauthenticationService) ReauthenticateWithCode(ctx context.Context, input inbound.ReauthenticateWithCodeInput) (inbound.ReauthenticateWithCodeResult, error) {
	if input.Subject == "" || input.SessionID == "" {
		return inbound.ReauthenticateWithCodeResult{}, outbound.ErrUnauthenticated
	}
	if s.repository == nil || s.guessSourceLimit == nil || s.guessAccountLimit == nil || len(s.verifierKey) != 32 {
		return inbound.ReauthenticateWithCodeResult{}, ErrReauthenticationUnavailable
	}
	var invalid bool
	var challengeExpiry, sessionExpiry time.Time
	window, err := s.repository.WithinCodeReauthenticationTransaction(ctx, func(tx outbound.CodeReauthenticationTransaction) error {
		challenge, bad, err := s.checkCode(ctx, tx, input)
		invalid = bad
		if err != nil || invalid {
			return err
		}
		consumed, err := tx.ConsumeChallenge(ctx, challenge.ID)
		if err != nil {
			return err
		}
		if !consumed {
			invalid = true
			return nil
		}
		sessionExpiry, err = openWindow(ctx, tx, input.Subject, input.SessionID)
		if err != nil {
			return err
		}
		challengeExpiry = challenge.ExpiresAt
		tx.RequireCommitBefore(challengeExpiry)
		return nil
	})
	if err != nil {
		return inbound.ReauthenticateWithCodeResult{}, codeReauthenticationCommitError(err, challengeExpiry, sessionExpiry)
	}

	if invalid {
		return inbound.ReauthenticateWithCodeResult{}, ErrInvalidReauthenticationCode
	}
	return inbound.ReauthenticateWithCodeResult{ExpiresAt: window.ExpiresAt()}, nil
}

func (s *CodeReauthenticationService) checkCode(ctx context.Context, tx outbound.CodeReauthenticationTransaction, input inbound.ReauthenticateWithCodeInput) (domain.Challenge, bool, error) {
	_, now, err := tx.LockSessionForReauthentication(ctx, input.Subject, input.SessionID)
	if err != nil {
		return domain.Challenge{}, false, err
	}
	account, err := tx.GetActiveAccountForSession(ctx, input.Subject, input.SessionID)
	if err != nil {
		return domain.Challenge{}, false, err
	}
	if err := s.guessSourceLimit(ctx, input.Source); err != nil {
		return domain.Challenge{}, false, err
	}
	if err := s.guessAccountLimit(ctx, input.Subject); err != nil {
		return domain.Challenge{}, false, err
	}
	if !account.EmailVerified || !validVerificationCode(input.Code) {
		return domain.Challenge{}, true, nil
	}
	challenge, found, err := tx.GetCurrentReauthenticationChallenge(ctx, input.Subject)
	if err != nil {
		return domain.Challenge{}, false, err
	}
	invalid, err := s.verifyCode(ctx, tx, input, account.Email, challenge, found, now)
	return challenge, invalid, err
}

func (s *CodeReauthenticationService) verifyCode(ctx context.Context, tx outbound.CodeReauthenticationTransaction, input inbound.ReauthenticateWithCodeInput, email string, challenge domain.Challenge, found bool, now time.Time) (bool, error) {
	if !found {
		return true, nil
	}
	check := challenge.Check(s.verifierKey, input.Subject, email, domain.PurposeReauthenticate, input.Code, now)
	if check == domain.CodeWrongGuess {
		return true, tx.IncrementChallengeWrongGuess(ctx, challenge.ID)
	}
	return check != domain.CodeAccepted, nil
}

func codeReauthenticationError(err error) error {
	switch {
	case err == nil, errors.Is(err, ErrInvalidReauthenticationCode), errors.Is(err, ErrReauthenticationEmailUnverified), errors.Is(err, ErrRateLimited), errors.Is(err, ErrLimitUnavailable), errors.Is(err, outbound.ErrUnauthenticated), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, outbound.ErrReauthenticationRequired):
		return outbound.ErrUnauthenticated
	default:
		return ErrReauthenticationUnavailable
	}
}

func codeReauthenticationCommitError(err error, challengeExpiry, sessionExpiry time.Time) error {
	if errors.Is(err, outbound.ErrReauthenticationRequired) && !challengeExpiry.IsZero() && challengeExpiry.Before(sessionExpiry) {
		return ErrInvalidReauthenticationCode
	}
	return codeReauthenticationError(err)
}
