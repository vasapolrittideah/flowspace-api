package domain

import "time"

type ReauthenticationWindow struct {
	AuthenticatedAt   time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
}

func (w ReauthenticationWindow) ExpiresAt() time.Time {
	if w.AuthenticatedAt.IsZero() {
		return time.Time{}
	}
	end := w.AuthenticatedAt.Add(10 * time.Minute)
	if w.IdleExpiresAt.Before(end) {
		end = w.IdleExpiresAt
	}
	if w.AbsoluteExpiresAt.Before(end) {
		end = w.AbsoluteExpiresAt
	}
	return end
}

func (w ReauthenticationWindow) IsOpen(now time.Time) bool {
	return !w.AuthenticatedAt.IsZero() && !now.Before(w.AuthenticatedAt) && now.Before(w.ExpiresAt())
}
