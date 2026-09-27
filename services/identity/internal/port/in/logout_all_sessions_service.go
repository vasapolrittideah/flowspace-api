package inbound

import "context"

type LogoutAllSessionsInput struct {
	Subject   string
	SessionID string
}

type LogoutAllSessionsService interface {
	LogoutAllSessions(ctx context.Context, input LogoutAllSessionsInput) error
}
