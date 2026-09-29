package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type refreshRepositoryStub struct {
	record           outbound.RefreshSessionRecord
	oldHash, newHash []byte
	replayed         bool
	err              error
}

func (s *refreshRepositoryStub) Rotate(_ context.Context, oldHash, newHash []byte, issue func(outbound.RefreshSessionRecord) error) (bool, error) {
	s.oldHash, s.newHash = append([]byte(nil), oldHash...), append([]byte(nil), newHash...)
	if s.err != nil || s.replayed {
		return s.replayed, s.err
	}
	return false, issue(s.record)
}

func TestRefreshSessionService(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 1)
	}
	current := base64.RawURLEncoding.EncodeToString(secret)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	repo := &refreshRepositoryStub{record: outbound.RefreshSessionRecord{
		Subject: "subject", ID: "session", IssuedAt: now,
		IdleExpiresAt: now.Add(30 * 24 * time.Hour), AbsoluteExpiresAt: now.Add(90 * 24 * time.Hour),
	}}
	signer := &tokenSignerStub{}
	service := NewSessionRefreshService(repo, signer)
	result, err := service.RefreshSession(context.Background(), current)
	if err != nil || result.AccessToken == "" || result.RefreshToken == "" || result.RefreshToken == current {
		t.Fatalf("refresh result = %+v, %v", result, err)
	}
	oldHash := sha256.Sum256(secret)
	newSecret, err := base64.RawURLEncoding.DecodeString(result.RefreshToken)
	if err != nil || len(newSecret) != 32 {
		t.Fatalf("new token length = %d, %v", len(newSecret), err)
	}
	newHash := sha256.Sum256(newSecret)
	if string(repo.oldHash) != string(oldHash[:]) || string(repo.newHash) != string(newHash[:]) {
		t.Fatal("repository did not receive token hashes")
	}
	if signer.claims.Subject != "subject" || signer.claims.SessionID != "session" || !signer.claims.IssuedAt.Equal(now) || !result.AccessTokenExpiresAt.Equal(now.Add(10*time.Minute)) || !result.SessionExpiresAt.Equal(repo.record.AbsoluteExpiresAt) {
		t.Fatal("wrong access claims or expiry")
	}
}

func TestRefreshSessionServiceFailures(t *testing.T) {
	current := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	repo := &refreshRepositoryStub{}
	service := NewSessionRefreshService(repo, &tokenSignerStub{})
	for _, invalid := range []string{"", "not-base64", base64.RawURLEncoding.EncodeToString(make([]byte, 31))} {
		if _, err := service.RefreshSession(context.Background(), invalid); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Fatalf("invalid token %q: %v", invalid, err)
		}
	}
	repo.replayed = true
	if result, err := service.RefreshSession(context.Background(), current); !errors.Is(err, ErrUnauthenticatedRefresh) || result.RefreshToken != "" {
		t.Fatalf("replay result = %+v, %v", result, err)
	}
	repo.replayed = false
	repo.err = errors.New("database unavailable")
	if _, err := service.RefreshSession(context.Background(), current); !errors.Is(err, ErrRefreshUnavailable) {
		t.Fatalf("store failure = %v", err)
	}
}
