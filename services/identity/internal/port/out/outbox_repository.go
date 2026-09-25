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
	Claim(context.Context, string) (OutboxEvent, bool, error)
	MarkPublished(context.Context, string, string) error
	Release(context.Context, string, string, time.Time) error
}
