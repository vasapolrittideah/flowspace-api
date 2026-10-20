package domain

import (
	"testing"
	"time"
)

func TestReauthenticationWindow(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	window := ReauthenticationWindow{AuthenticatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)}
	for _, test := range []struct {
		name string
		at   time.Time
		open bool
	}{
		{"before proof", now.Add(-time.Nanosecond), false},
		{"at proof", now, true},
		{"before end", now.Add(10*time.Minute - time.Nanosecond), true},
		{"at end", now.Add(10 * time.Minute), false},
		{"after end", now.Add(11 * time.Minute), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := window.IsOpen(test.at); got != test.open {
				t.Fatalf("open = %t, want %t", got, test.open)
			}
		})
	}
	if got := window.ExpiresAt(); !got.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("expiry = %v", got)
	}
	window.IdleExpiresAt = now.Add(time.Minute)
	if got := window.ExpiresAt(); !got.Equal(window.IdleExpiresAt) || window.IsOpen(got) {
		t.Fatalf("idle expiry = %v", got)
	}
	window.AbsoluteExpiresAt = now.Add(30 * time.Second)
	if got := window.ExpiresAt(); !got.Equal(window.AbsoluteExpiresAt) || window.IsOpen(got) {
		t.Fatalf("absolute expiry = %v", got)
	}
	window.AuthenticatedAt = time.Time{}
	if window.IsOpen(now) || !window.ExpiresAt().IsZero() {
		t.Fatal("legacy session has an open window")
	}
}
