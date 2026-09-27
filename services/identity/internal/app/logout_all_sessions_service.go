package app

import (
	"context"
	"errors"

	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrAllLogoutUnavailable = errors.New("all-session logout unavailable")

type LogoutAllSessionsService struct {
	repository outbound.AllSessionLogoutRepository
}

var _ inbound.LogoutAllSessionsService = (*LogoutAllSessionsService)(nil)

func NewLogoutAllSessionsService(repository outbound.AllSessionLogoutRepository) *LogoutAllSessionsService {
	return &LogoutAllSessionsService{repository: repository}
}

func (s *LogoutAllSessionsService) LogoutAllSessions(ctx context.Context, input inbound.LogoutAllSessionsInput) error {
	var revoke func(context.Context, string, string) error
	if s.repository != nil {
		revoke = s.repository.RevokeAll
	}
	return revokeSessions(ctx, input.Subject, input.SessionID, revoke, ErrAllLogoutUnavailable)
}
