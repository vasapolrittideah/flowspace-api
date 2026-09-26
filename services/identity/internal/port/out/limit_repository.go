package outbound

import (
	"context"
	"time"
)

type LimitRepository interface {
	Record(ctx context.Context, scope, key, action string, maximum, dailyMaximum int, window, interval time.Duration) (bool, error)
	RecordLogin(ctx context.Context, source, emailKey string, sourceMaximum, emailMaximum int, sourceWindow, emailWindow time.Duration) (bool, error)
}
