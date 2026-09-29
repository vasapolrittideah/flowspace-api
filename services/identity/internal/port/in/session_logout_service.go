package inbound

import (
	"context"
)

type LogoutCurrentSessionInput struct {
	Subject   string
	SessionID string
}

type CurrentSessionLogoutService interface {
	LogoutCurrentSession(ctx context.Context, input LogoutCurrentSessionInput) error
}

type LogoutAllSessionsInput struct {
	Subject   string
	SessionID string
}

type AllSessionLogoutService interface {
	LogoutAllSessions(ctx context.Context, input LogoutAllSessionsInput) error
}
