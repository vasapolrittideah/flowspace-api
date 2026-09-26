package app

import (
	"context"
	"errors"

	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrSessionCheckUnavailable = errors.New("session check unavailable")

type SessionCheckService struct {
	repository outbound.SessionCheckRepository
}

var _ inbound.SessionCheckService = (*SessionCheckService)(nil)

func NewSessionCheckService(repository outbound.SessionCheckRepository) *SessionCheckService {
	return &SessionCheckService{repository: repository}
}

func (s *SessionCheckService) CheckSession(ctx context.Context, input inbound.CheckSessionInput) (bool, error) {
	if input.Subject == "" || input.SessionID == "" {
		return false, outbound.ErrUnauthenticated
	}
	if s.repository == nil {
		return false, ErrSessionCheckUnavailable
	}
	verified, err := s.repository.Check(ctx, input.Subject, input.SessionID)
	switch {
	case err == nil:
		return verified, nil
	case errors.Is(err, outbound.ErrUnauthenticated), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return false, err
	default:
		return false, ErrSessionCheckUnavailable
	}
}
