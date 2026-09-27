package outbound

import "context"

type AllSessionLogoutRepository interface {
	RevokeAll(ctx context.Context, subject, sessionID string) error
}
