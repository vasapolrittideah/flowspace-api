package token

import (
	"crypto/ed25519"
	"errors"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

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
	token, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.EdDSA})
	if err != nil || len(token.Headers) != 1 ||
		token.Headers[0].ExtraHeaders[jose.HeaderKey("typ")] != "at+jwt" {
		return outbound.AccessTokenIdentity{}, outbound.ErrUnauthenticated
	}
	key := v.keys[token.Headers[0].KeyID]
	if len(key) != ed25519.PublicKeySize {
		return outbound.AccessTokenIdentity{}, outbound.ErrUnauthenticated
	}
	var standard jwt.Claims
	var extra struct {
		SessionID string `json:"sid"`
	}
	if err := token.Claims(key, &standard, &extra); err != nil || standard.Subject == "" || extra.SessionID == "" ||
		standard.ID == "" || standard.IssuedAt == nil || standard.Expiry == nil ||
		standard.ValidateWithLeeway(jwt.Expected{Issuer: v.issuer, AnyAudience: jwt.Audience{v.audience}}, 0) != nil {
		return outbound.AccessTokenIdentity{}, outbound.ErrUnauthenticated
	}
	return outbound.AccessTokenIdentity{Subject: standard.Subject, SessionID: extra.SessionID}, nil
}
