package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type reauthenticationTransactionStub struct {
	window   domain.ReauthenticationWindow
	now      time.Time
	err      error
	deadline time.Time
}

func (s *reauthenticationTransactionStub) LockSessionForReauthentication(context.Context, string, string) (domain.ReauthenticationWindow, time.Time, error) {
	return s.window, s.now, s.err
}

func (s *reauthenticationTransactionStub) RequireCommitBefore(deadline time.Time) {
	s.deadline = deadline
}

func TestRequireReauthentication(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	tx := &reauthenticationTransactionStub{window: domain.ReauthenticationWindow{AuthenticatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)}, now: now}
	if err := RequireReauthentication(t.Context(), tx, "subject", "session"); err != nil {
		t.Fatal(err)
	}
	if !tx.deadline.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("guard registered deadline %v", tx.deadline)
	}
	tx.now = now.Add(11 * time.Minute)
	if err := RequireReauthentication(t.Context(), tx, "subject", "session"); !errors.Is(err, ErrReauthenticationRequired) {
		t.Fatalf("closed window = %v", err)
	}
	// A refresh can move the idle expiry but does not move the proof time.
	tx.window.IdleExpiresAt = now.Add(30 * 24 * time.Hour)
	if err := RequireReauthentication(t.Context(), tx, "subject", "session"); !errors.Is(err, ErrReauthenticationRequired) {
		t.Fatalf("refresh reopened the window: %v", err)
	}
}

func TestRequireReauthenticationFailures(t *testing.T) {
	for _, test := range []struct{ err, want error }{
		{outbound.ErrUnauthenticated, outbound.ErrUnauthenticated},
		{context.Canceled, context.Canceled},
		{context.DeadlineExceeded, context.DeadlineExceeded},
		{errors.New("database detail"), ErrReauthenticationUnavailable},
	} {
		tx := &reauthenticationTransactionStub{err: test.err}
		if err := RequireReauthentication(t.Context(), tx, "subject", "session"); !errors.Is(err, test.want) || !tx.deadline.IsZero() {
			t.Fatalf("guard error = %v, want %v", err, test.want)
		}
	}
	for _, ids := range [][2]string{{"", "session"}, {"subject", ""}} {
		if err := RequireReauthentication(t.Context(), nil, ids[0], ids[1]); !errors.Is(err, outbound.ErrUnauthenticated) {
			t.Fatalf("empty identity = %v", err)
		}
	}
	if err := RequireReauthentication(t.Context(), nil, "subject", "session"); !errors.Is(err, ErrReauthenticationUnavailable) {
		t.Fatalf("missing transaction = %v", err)
	}
}
