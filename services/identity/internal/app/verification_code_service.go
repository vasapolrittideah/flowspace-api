package app

import (
	"context"
	"errors"

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
