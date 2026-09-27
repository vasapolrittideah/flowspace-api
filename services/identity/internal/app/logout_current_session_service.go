package app

import (
	"context"
	"errors"

	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrCurrentLogoutUnavailable = errors.New("current session logout unavailable")

type LogoutCurrentSessionService struct {
	repository outbound.CurrentSessionLogoutRepository
}

var _ inbound.LogoutCurrentSessionService = (*LogoutCurrentSessionService)(nil)

func NewLogoutCurrentSessionService(repository outbound.CurrentSessionLogoutRepository) *LogoutCurrentSessionService {
	return &LogoutCurrentSessionService{repository: repository}
}

func (s *LogoutCurrentSessionService) LogoutCurrentSession(ctx context.Context, input inbound.LogoutCurrentSessionInput) error {
	if input.Subject == "" || input.SessionID == "" {
		return outbound.ErrUnauthenticated
	}
	if s.repository == nil {
		return ErrCurrentLogoutUnavailable
	}
	err := s.repository.RevokeCurrent(ctx, input.Subject, input.SessionID)
	switch {
	case err == nil, errors.Is(err, outbound.ErrUnauthenticated), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return ErrCurrentLogoutUnavailable
	}
}
