package app

import (
	"context"
	"errors"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var (
	ErrInvalidPasswordResetCode = errors.New("invalid password reset code")
	ErrPasswordResetUnavailable = errors.New("password reset unavailable")
)

type PasswordResetService struct {
	repository   outbound.PasswordResetRepository
	sourceLimit  func(context.Context, string) error
	emailLimit   func(context.Context, string) error
	accountLimit func(context.Context, string) error
	compromised  func(context.Context, string) (bool, error)
	verifierKey  []byte
}

var _ inbound.PasswordResetService = (*PasswordResetService)(nil)

func NewPasswordResetService(repository outbound.PasswordResetRepository,
	sourceLimit, emailLimit, accountLimit func(context.Context, string) error,
	compromised func(context.Context, string) (bool, error), verifierKey []byte,
) *PasswordResetService {
	return &PasswordResetService{
		repository: repository, sourceLimit: sourceLimit, emailLimit: emailLimit,
		accountLimit: accountLimit, compromised: compromised, verifierKey: verifierKey,
	}
}

func (s *PasswordResetService) ResetPassword(ctx context.Context, input inbound.ResetPasswordInput) error {
	email, err := s.validateResetRequest(ctx, input)
	if err != nil {
		return err
	}
	account, found, err := s.repository.FindPasswordAccount(ctx, email)
	if err != nil {
		return resetFailure(err)
	}
	if found {
		if err := s.accountLimit(ctx, account.Subject); err != nil {
			return err
		}
	}
	invalid := false
	err = s.repository.WithinPasswordResetTransaction(ctx, func(tx outbound.PasswordResetTransaction) error {
		var stepErr error
		invalid, stepErr = s.reset(ctx, tx, email, input.Code, input.NewPassword)
		return stepErr
	})
	if err != nil {
		return resetFailure(err)
	}
	if invalid {
		return ErrInvalidPasswordResetCode
	}
	return nil
}

func (s *PasswordResetService) validateResetRequest(ctx context.Context, input inbound.ResetPasswordInput) (string, error) {
	if s.repository == nil || s.sourceLimit == nil || s.emailLimit == nil || s.accountLimit == nil || s.compromised == nil || len(s.verifierKey) != 32 {
		return "", ErrPasswordResetUnavailable
	}
	if err := s.sourceLimit(ctx, input.Source); err != nil {
		return "", err
	}
	email, err := domain.NormalizeEmail(input.Email)
	if err != nil {
		return "", err
	}
	if err := s.emailLimit(ctx, email); err != nil {
		return "", err
	}
	if _, err := domain.ValidatePassword(input.NewPassword); err != nil {
		return "", err
	}
	if !validVerificationCode(input.Code) {
		return "", ErrInvalidPasswordResetCode
	}
	return email, nil
}

func (s *PasswordResetService) reset(ctx context.Context, tx outbound.PasswordResetTransaction, email, code, password string) (bool, error) {
	subject, challenge, invalid, err := s.verifyResetCode(ctx, tx, email, code)
	if invalid || err != nil {
		return invalid, err
	}
	hash, err := domain.HashPassword(ctx, password, s.compromised)
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	consumed, err := tx.ConsumeChallenge(ctx, challenge.ID)
	if err != nil || !consumed {
		return !consumed, err
	}
	return false, applyPasswordReset(ctx, tx, subject, challenge.ID, hash)
}

func (s *PasswordResetService) verifyResetCode(ctx context.Context, tx outbound.PasswordResetTransaction, email, code string) (string, outbound.ChallengeState, bool, error) {
	account, found, err := tx.GetAccountForPasswordRecovery(ctx, email)
	if err != nil {
		return "", outbound.ChallengeState{}, false, err
	}
	challenge := outbound.ChallengeState{}
	current := false
	eligible := found && account.EmailVerified && account.HasPassword
	if eligible {
		challenge, current, err = tx.GetCurrentPasswordResetChallenge(ctx, account.Subject)
		if err != nil {
			return "", outbound.ChallengeState{}, false, err
		}
	}
	subject := account.Subject
	if subject == "" {
		subject = "unknown-password-reset-account"
	}
	// Always perform one keyed verifier comparison for a well-formed code.
	matched := domain.VerifyChallenge(s.verifierKey, subject, email, domain.PurposePasswordReset,
		code, challenge.Verifier, time.Now().Add(time.Minute), time.Now())
	usable := current && challenge.Email == email && challenge.WrongGuesses < 5 && time.Now().Before(challenge.ExpiresAt)
	if eligible && usable && matched {
		return account.Subject, challenge, false, nil
	}
	if usable {
		return "", outbound.ChallengeState{}, true, tx.IncrementChallengeWrongGuess(ctx, challenge.ID)
	}
	return "", outbound.ChallengeState{}, true, nil
}

func applyPasswordReset(ctx context.Context, tx outbound.PasswordResetTransaction, subject, challengeID, hash string) error {
	if err := tx.ReplacePasswordResetChallenge(ctx, subject); err != nil {
		return err
	}
	if err := tx.DeleteChallengeDelivery(ctx, challengeID); err != nil {
		return err
	}
	if err := tx.UpdatePasswordHash(ctx, subject, hash); err != nil {
		return err
	}
	if err := tx.RevokePasswordSessions(ctx, subject); err != nil {
		return err
	}
	return tx.QueuePasswordChangeNotice(ctx, subject)
}

func resetFailure(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidEmail), errors.Is(err, domain.ErrInvalidPassword),
		errors.Is(err, ErrRateLimited), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return ErrPasswordResetUnavailable
	}
}
