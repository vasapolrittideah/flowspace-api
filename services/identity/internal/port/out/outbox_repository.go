package outbound

import (
	"context"
	"time"
)

// OutboxEvent is a claimed outbox event. Traceparent and Tracestate hold the
// W3C trace context of the transaction that inserted the event. They are
// empty when that transaction had no valid span.
type OutboxEvent struct {
	ID          string
	ChallengeID string
	Purpose     string
	Traceparent string
	Tracestate  string
}

type OutboxRepository interface {
	Claim(ctx context.Context, owner string) (OutboxEvent, bool, error)
	MarkPublished(ctx context.Context, eventID, owner string) error
	Release(ctx context.Context, eventID, owner string, nextAttempt time.Time) error
}
