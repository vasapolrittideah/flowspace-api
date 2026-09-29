package outbound

import (
	"context"
)

type CurrentSessionLogoutRepository interface {
	RevokeCurrent(ctx context.Context, subject, sessionID string) error
}

type AllSessionLogoutRepository interface {
	RevokeAll(ctx context.Context, subject, sessionID string) error
}
