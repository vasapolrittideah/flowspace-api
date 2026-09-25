package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

type sessionRepositoryStub struct {
	hash   []byte
	record SessionRecord
}

func (s *sessionRepositoryStub) Create(_ context.Context, _ string, hash []byte) (SessionRecord, error) {
	s.hash = append([]byte(nil), hash...)
	return s.record, nil
}

type tokenSignerStub struct {
	claims AccessTokenClaims
	err    error
}

func (s *tokenSignerStub) Sign(claims AccessTokenClaims) (string, error) {
	s.claims = claims
	return "signed-access-token", s.err
}

func TestSessionServiceIssue(t *testing.T) {
	created := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	repository := &sessionRepositoryStub{record: SessionRecord{
		ID: "session-id", CreatedAt: created,
		IdleExpiresAt:     created.Add(30 * 24 * time.Hour),
		AbsoluteExpiresAt: created.Add(90 * 24 * time.Hour),
	}}
	signer := &tokenSignerStub{}
	issued, err := NewSessionService(repository, signer).Issue(context.Background(), "subject-1")
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := base64.RawURLEncoding.DecodeString(issued.RefreshToken)
	if err != nil || len(refresh) != 32 {
		t.Fatalf("refresh token has %d random bytes: %v", len(refresh), err)
	}
	hash := sha256.Sum256(refresh)
	if string(repository.hash) != string(hash[:]) || string(repository.hash) == issued.RefreshToken {
		t.Fatal("repository did not receive only the refresh-token hash")
	}
	if issued.AccessToken != "signed-access-token" || !issued.AccessTokenExpiresAt.Equal(created.Add(10*time.Minute)) || !issued.RefreshTokenExpiresAt.Equal(repository.record.IdleExpiresAt) || !issued.SessionExpiresAt.Equal(repository.record.AbsoluteExpiresAt) {
		t.Fatal("wrong token response or expiry")
	}
	if signer.claims.Subject != "subject-1" || signer.claims.SessionID != "session-id" || signer.claims.IssuedAt != created || signer.claims.ExpiresAt != issued.AccessTokenExpiresAt || signer.claims.ID == "" {
		t.Fatalf("wrong access claims: %+v", signer.claims)
	}
}

func TestSessionServiceCapsAccessAtAbsoluteExpiry(t *testing.T) {
	created := time.Now().UTC()
	repository := &sessionRepositoryStub{record: SessionRecord{ID: "session-id", CreatedAt: created, IdleExpiresAt: created.Add(time.Minute), AbsoluteExpiresAt: created.Add(time.Minute)}}
	signer := &tokenSignerStub{}
	issued, err := NewSessionService(repository, signer).Issue(context.Background(), "subject-1")
	if err != nil || !issued.AccessTokenExpiresAt.Equal(repository.record.AbsoluteExpiresAt.Truncate(time.Second)) {
		t.Fatalf("access expiry = %v, %v", issued.AccessTokenExpiresAt, err)
	}
}

func TestSessionServiceDoesNotReturnSecretsOnSigningError(t *testing.T) {
	created := time.Now().UTC()
	repository := &sessionRepositoryStub{record: SessionRecord{ID: "session-id", CreatedAt: created, IdleExpiresAt: created.Add(30 * 24 * time.Hour), AbsoluteExpiresAt: created.Add(90 * 24 * time.Hour)}}
	signer := &tokenSignerStub{err: errors.New("signing failed")}
	issued, err := NewSessionService(repository, signer).Issue(context.Background(), "subject-1")
	if err == nil || issued.RefreshToken != "" || issued.AccessToken != "" {
		t.Fatal("signing failure returned tokens or no error")
	}
}
