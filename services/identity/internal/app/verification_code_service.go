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
	ErrEmailAlreadyVerified        = errors.New("email already verified")
	ErrVerificationCodeUnavailable = errors.New("verification code unavailable")
	ErrInvalidVerificationCode     = errors.New("invalid verification code")
)

type VerificationCodeService struct {
	repository        outbound.VerificationCodeRepository
	protector         outbound.DeliveryProtector
	sourceLimit       func(context.Context, string) error
	guessSourceLimit  func(context.Context, string) error
	guessAccountLimit func(context.Context, string) error
	verifierKey       []byte
}

var _ inbound.VerificationCodeService = (*VerificationCodeService)(nil)

func NewVerificationCodeService(repository outbound.VerificationCodeRepository, protector outbound.DeliveryProtector,
	sourceLimit, guessSourceLimit, guessAccountLimit func(context.Context, string) error, verifierKey []byte,
) *VerificationCodeService {
	return &VerificationCodeService{
		repository: repository, protector: protector, sourceLimit: sourceLimit,
		guessSourceLimit: guessSourceLimit, guessAccountLimit: guessAccountLimit, verifierKey: verifierKey,
	}
}

func (s *VerificationCodeService) VerifyEmail(ctx context.Context, input inbound.VerifyEmailInput) error {
	if input.Subject == "" || input.SessionID == "" {
		return outbound.ErrUnauthenticated
	}
	if s.repository == nil || s.guessSourceLimit == nil || s.guessAccountLimit == nil || len(s.verifierKey) != 32 {
		return ErrVerificationCodeUnavailable
	}
	if err := s.guessSourceLimit(ctx, input.Source); err != nil {
		return err
	}
	if err := s.guessAccountLimit(ctx, input.Subject); err != nil {
		return err
	}
	invalid := false
	err := s.repository.WithinVerificationTransaction(ctx, func(tx outbound.VerificationTransaction) error {
		var stepErr error
		invalid, stepErr = s.verify(ctx, tx, input)
		return stepErr
	})
	switch {
	case err == nil && invalid:
		return ErrInvalidVerificationCode
	case err == nil, errors.Is(err, outbound.ErrUnauthenticated), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return ErrVerificationCodeUnavailable
	}
}

func (s *VerificationCodeService) verify(ctx context.Context, tx outbound.VerificationTransaction, input inbound.VerifyEmailInput) (bool, error) {
	account, err := tx.GetActiveAccountForSession(ctx, input.Subject, input.SessionID)
	if err != nil {
		return false, err
	}
	if !validVerificationCode(input.Code) {
		return true, nil
	}
	if account.EmailVerified {
		return false, nil
	}
	challenge, found, err := tx.GetCurrentVerificationChallenge(ctx, input.Subject)
	if err != nil {
		return false, err
	}
	return s.consume(ctx, tx, input, account.Email, challenge, found)
}

func (s *VerificationCodeService) consume(ctx context.Context, tx outbound.VerificationTransaction, input inbound.VerifyEmailInput,
	email string, challenge outbound.ChallengeState, found bool,
) (bool, error) {
	if !found || challenge.Email != email || challenge.WrongGuesses >= 5 ||
		!domain.VerifyChallenge(s.verifierKey, input.Subject, email, domain.PurposeVerifyEmail,
			input.Code, challenge.Verifier, challenge.ExpiresAt, time.Now()) {
		if found && challenge.Email == email && challenge.WrongGuesses < 5 && time.Now().Before(challenge.ExpiresAt) {
			return true, tx.IncrementChallengeWrongGuess(ctx, challenge.ID)
		}
		return true, nil
	}
	consumed, err := tx.ConsumeChallenge(ctx, challenge.ID)
	if err != nil {
		return false, err
	}
	if !consumed {
		return true, nil
	}
	changed, err := tx.MarkEmailVerified(ctx, input.Subject)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, ErrVerificationCodeUnavailable
	}
	return false, nil
}

func validVerificationCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for i := range len(code) {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}

func (s *VerificationCodeService) RequestEmailVerificationCode(ctx context.Context, input inbound.RequestEmailVerificationCodeInput) error {
	if input.Subject == "" || input.SessionID == "" {
		return outbound.ErrUnauthenticated
	}
	if s.repository == nil || s.protector == nil || s.sourceLimit == nil || len(s.verifierKey) != 32 {
		return ErrVerificationCodeUnavailable
	}
	if err := s.sourceLimit(ctx, input.Source); err != nil {
		return err
	}
	err := s.repository.WithinTransaction(ctx, func(tx outbound.AccountTransaction) error {
		return s.issue(ctx, tx, input)
	})
	switch {
	case err == nil, errors.Is(err, outbound.ErrUnauthenticated), errors.Is(err, ErrEmailAlreadyVerified), errors.Is(err, ErrRateLimited),
		errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return ErrVerificationCodeUnavailable
	}
}

func (s *VerificationCodeService) issue(ctx context.Context, tx outbound.AccountTransaction, input inbound.RequestEmailVerificationCodeInput) error {
	account, err := tx.GetActiveAccountForSession(ctx, input.Subject, input.SessionID)
	if err != nil {
		return err
	}
	if account.EmailVerified {
		return ErrEmailAlreadyVerified
	}
	allowed, err := tx.CanIssueCode(ctx, input.Subject)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}
	code, verifier, _, err := domain.NewChallenge(s.verifierKey, input.Subject, account.Email, domain.PurposeVerifyEmail, time.Now())
	if err != nil {
		return err
	}
	if err := tx.ReplaceVerificationChallenge(ctx, input.Subject); err != nil {
		return err
	}
	challengeID, err := tx.CreateChallenge(ctx, input.Subject, account.Email, verifier)
	if err != nil {
		return err
	}
	material, err := s.protector.Protect(challengeID, string(domain.PurposeVerifyEmail), input.Subject, account.Email, code)
	if err != nil {
		return err
	}
	if err := tx.StoreDelivery(ctx, challengeID, material); err != nil {
		return err
	}
	return tx.CreateOutboxEvent(ctx, challengeID)
}
