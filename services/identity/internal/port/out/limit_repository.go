package outbound

import (
	"context"
	"time"
)

type LimitRepository interface {
	Record(ctx context.Context, scope, key, action string, maximum, dailyMaximum int, window, interval time.Duration) (bool, error)
}
