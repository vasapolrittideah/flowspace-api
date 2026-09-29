package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var (
	ErrInvalidRefreshToken    = errors.New("invalid refresh token")
	ErrUnauthenticatedRefresh = errors.New("refresh token not authenticated")
	ErrRefreshUnavailable     = errors.New("refresh unavailable")
)

type SessionRefreshService struct {
	repository outbound.SessionRefreshRepository
	signer     outbound.TokenSigner
}

var _ inbound.SessionRefreshService = (*SessionRefreshService)(nil)

func NewSessionRefreshService(repository outbound.SessionRefreshRepository, signer outbound.TokenSigner) *SessionRefreshService {
	return &SessionRefreshService{repository: repository, signer: signer}
}

func (s *SessionRefreshService) RefreshSession(ctx context.Context, raw string) (inbound.RefreshSessionResult, error) {
	if s.repository == nil || s.signer == nil {
		return inbound.RefreshSessionResult{}, ErrRefreshUnavailable
	}
	secret, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(secret) != 32 || base64.RawURLEncoding.EncodeToString(secret) != raw {
		return inbound.RefreshSessionResult{}, ErrInvalidRefreshToken
	}
	oldHash := sha256.Sum256(secret)
	next := make([]byte, 32)
	if _, err := rand.Read(next); err != nil {
		return inbound.RefreshSessionResult{}, ErrRefreshUnavailable
	}
	newHash := sha256.Sum256(next)
	var result inbound.RefreshSessionResult
	replayed, err := s.repository.Rotate(ctx, oldHash[:], newHash[:], func(session outbound.RefreshSessionRecord) error {
		jti := make([]byte, 16)
		if _, err := rand.Read(jti); err != nil {
			return err
		}
		expires := session.IssuedAt.Add(10 * time.Minute)
		if expires.After(session.AbsoluteExpiresAt) {
			expires = session.AbsoluteExpiresAt
		}
		expires = expires.Truncate(time.Second)
		access, err := s.signer.Sign(outbound.AccessTokenClaims{
			Subject: session.Subject, SessionID: session.ID, ID: base64.RawURLEncoding.EncodeToString(jti),
			IssuedAt: session.IssuedAt, ExpiresAt: expires,
		})
		if err != nil {
			return err
		}
		result = inbound.RefreshSessionResult{
			AccessToken: access, RefreshToken: base64.RawURLEncoding.EncodeToString(next),
			AccessTokenExpiresAt: expires, RefreshTokenExpiresAt: session.IdleExpiresAt,
			SessionExpiresAt: session.AbsoluteExpiresAt,
		}
		return nil
	})
	if errors.Is(err, outbound.ErrUnauthenticated) {
		return inbound.RefreshSessionResult{}, ErrUnauthenticatedRefresh
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return inbound.RefreshSessionResult{}, err
		}
		return inbound.RefreshSessionResult{}, ErrRefreshUnavailable
	}
	if replayed {
		return inbound.RefreshSessionResult{}, ErrUnauthenticatedRefresh
	}
	return result, nil
}
