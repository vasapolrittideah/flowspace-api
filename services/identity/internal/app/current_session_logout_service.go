package app

import (
	"context"
	"errors"

	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrCurrentLogoutUnavailable = errors.New("current session logout unavailable")

type CurrentSessionLogoutService struct {
	repository outbound.CurrentSessionLogoutRepository
}

var _ inbound.CurrentSessionLogoutService = (*CurrentSessionLogoutService)(nil)

func NewCurrentSessionLogoutService(repository outbound.CurrentSessionLogoutRepository) *CurrentSessionLogoutService {
	return &CurrentSessionLogoutService{repository: repository}
}

func (s *CurrentSessionLogoutService) LogoutCurrentSession(ctx context.Context, input inbound.LogoutCurrentSessionInput) error {
	var revoke func(context.Context, string, string) error
	if s.repository != nil {
		revoke = s.repository.RevokeCurrent
	}
	return revokeSessions(ctx, input.Subject, input.SessionID, revoke, ErrCurrentLogoutUnavailable)
}

func revokeSessions(ctx context.Context, subject, sessionID string, revoke func(context.Context, string, string) error, unavailable error) error {
	if subject == "" || sessionID == "" {
		return outbound.ErrUnauthenticated
	}
	if revoke == nil {
		return unavailable
	}
	err := revoke(ctx, subject, sessionID)
	switch {
	case err == nil, errors.Is(err, outbound.ErrUnauthenticated), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	default:
		return unavailable
	}
}
