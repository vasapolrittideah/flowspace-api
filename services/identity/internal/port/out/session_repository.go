package outbound

import (
	"context"
	"time"
)

type SessionRecord struct {
	ID                string
	CreatedAt         time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
}

type SessionRepository interface {
	Create(ctx context.Context, subject string, hash []byte) (SessionRecord, error)
}
