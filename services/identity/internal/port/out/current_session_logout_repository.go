package outbound

import "context"

type CurrentSessionLogoutRepository interface {
	RevokeCurrent(ctx context.Context, subject, sessionID string) error
}
