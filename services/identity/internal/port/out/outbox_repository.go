package outbound

import (
	"context"
	"time"
)

type OutboxEvent struct {
	ID          string
	ChallengeID string
	Purpose     string
}

type OutboxRepository interface {
	Claim(ctx context.Context, owner string) (OutboxEvent, bool, error)
	MarkPublished(ctx context.Context, eventID, owner string) error
	Release(ctx context.Context, eventID, owner string, nextAttempt time.Time) error
}
