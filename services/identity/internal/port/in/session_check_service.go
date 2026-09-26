package inbound

import "context"

type CheckSessionInput struct {
	Subject   string
	SessionID string
}

type SessionCheckService interface {
	CheckSession(ctx context.Context, input CheckSessionInput) (bool, error)
}
