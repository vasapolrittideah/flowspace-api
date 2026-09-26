package outbound

import "context"

type SessionCheckRepository interface {
	Check(ctx context.Context, subject, sessionID string) (bool, error)
}
