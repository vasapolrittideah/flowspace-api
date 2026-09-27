package inbound

import "context"

type LogoutCurrentSessionInput struct {
	Subject   string
	SessionID string
}

type LogoutCurrentSessionService interface {
	LogoutCurrentSession(ctx context.Context, input LogoutCurrentSessionInput) error
}
