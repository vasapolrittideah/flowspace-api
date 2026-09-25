package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"
)

var ErrSessionIssue = errors.New("session issuance failed")

type SessionRecord struct {
	ID                string
	CreatedAt         time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
}

type SessionRepository interface {
	Create(ctx context.Context, subject string, hash []byte) (SessionRecord, error)
}

type AccessTokenClaims struct {
	Subject   string
	SessionID string
	ID        string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type TokenSigner interface {
	Sign(claims AccessTokenClaims) (string, error)
}

type SessionTokens struct {
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
	SessionExpiresAt      time.Time
}

type SessionService struct {
	repository SessionRepository
	signer     TokenSigner
}

func NewSessionService(repository SessionRepository, signer TokenSigner) *SessionService {
	return &SessionService{repository: repository, signer: signer}
}

// Issue requires a repository bound to the caller's account transaction.
func (s *SessionService) Issue(ctx context.Context, subject string) (SessionTokens, error) {
	if subject == "" {
		return SessionTokens{}, ErrSessionIssue
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return SessionTokens{}, ErrSessionIssue
	}
	hash := sha256.Sum256(secret)
	session, err := s.repository.Create(ctx, subject, hash[:])
	if err != nil {
		return SessionTokens{}, ErrSessionIssue
	}
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return SessionTokens{}, ErrSessionIssue
	}
	expires := session.CreatedAt.Add(10 * time.Minute)
	if expires.After(session.AbsoluteExpiresAt) {
		expires = session.AbsoluteExpiresAt
	}
	expires = expires.Truncate(time.Second)
	access, err := s.signer.Sign(AccessTokenClaims{
		Subject: subject, SessionID: session.ID, ID: base64.RawURLEncoding.EncodeToString(jti),
		IssuedAt: session.CreatedAt, ExpiresAt: expires,
	})
	if err != nil {
		return SessionTokens{}, ErrSessionIssue
	}
	return SessionTokens{
		AccessToken: access, RefreshToken: base64.RawURLEncoding.EncodeToString(secret),
		AccessTokenExpiresAt: expires, RefreshTokenExpiresAt: session.IdleExpiresAt,
		SessionExpiresAt: session.AbsoluteExpiresAt,
	}, nil
}
