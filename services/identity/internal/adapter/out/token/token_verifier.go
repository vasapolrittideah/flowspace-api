package token

import (
	"crypto/ed25519"
	"errors"

	"github.com/vasapolrittideah/flowspace-api/internal/authn"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type Verifier struct {
	keys     map[string]ed25519.PublicKey
	issuer   string
	audience string
}

var _ outbound.AccessTokenVerifier = (*Verifier)(nil)

func NewVerifier(key ed25519.PublicKey, keyID, issuer, audience string) (*Verifier, error) {
	return NewVerifierKeys(map[string]ed25519.PublicKey{keyID: key}, issuer, audience)
}

func NewVerifierKeys(keys map[string]ed25519.PublicKey, issuer, audience string) (*Verifier, error) {
	if len(keys) == 0 || issuer == "" || audience == "" {
		return nil, errors.New("invalid token verifier configuration")
	}
	copyKeys := make(map[string]ed25519.PublicKey, len(keys))
	for keyID, key := range keys {
		if keyID == "" || len(key) != ed25519.PublicKeySize {
			return nil, errors.New("invalid token verifier configuration")
		}
		copyKeys[keyID] = append(ed25519.PublicKey(nil), key...)
	}
	return &Verifier{keys: copyKeys, issuer: issuer, audience: audience}, nil
}

func (v *Verifier) Verify(raw string) (outbound.AccessTokenIdentity, error) {
	claims, err := authn.VerifyAccessToken(raw, v.issuer, v.audience, func(keyID string) (ed25519.PublicKey, error) {
		return v.keys[keyID], nil
	})
	if err != nil {
		return outbound.AccessTokenIdentity{}, outbound.ErrUnauthenticated
	}
	return outbound.AccessTokenIdentity{Subject: claims.Subject, SessionID: claims.SessionID}, nil
}
