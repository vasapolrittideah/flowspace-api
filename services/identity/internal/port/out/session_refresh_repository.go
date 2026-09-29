package outbound

import (
	"context"
	"time"
)

type RefreshSessionRecord struct {
	Subject           string
	ID                string
	IssuedAt          time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
}

type SessionRefreshRepository interface {
	Rotate(ctx context.Context, oldHash, newHash []byte, issue func(RefreshSessionRecord) error) (replayed bool, err error)
}
