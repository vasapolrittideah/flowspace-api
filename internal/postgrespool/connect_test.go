package postgrespool

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	attempts := 0
	err := retry(ctx, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary connection failure")
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("retry() = %v after %d attempts, want success after 3", err, attempts)
	}

	canceled, stop := context.WithCancel(t.Context())
	stop()
	if err := retry(canceled, func(context.Context) error { return errors.New("unavailable") }); !errors.Is(err, context.Canceled) {
		t.Fatalf("retry() with canceled context = %v, want context.Canceled", err)
	}
}

func TestOpenStopsAfterDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	pool, err := Open(ctx, "postgres://test:private-value@127.0.0.1:1/test?sslmode=disable")
	if pool != nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open() = %v, %v, want nil, ErrUnavailable", pool, err)
	}
	if strings.Contains(err.Error(), "private-value") {
		t.Fatal("Open() exposed the database password")
	}
}

func TestOpenRejectsInvalidConfiguration(t *testing.T) {
	pool, err := Open(t.Context(), "postgres://%")
	if pool != nil || !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("Open() = %v, %v, want nil, ErrInvalidConfiguration", pool, err)
	}
}
