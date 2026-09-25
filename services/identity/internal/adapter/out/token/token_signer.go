package token

import (
	"crypto/ed25519"
	"errors"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
)

type Signer struct {
	signer   jose.Signer
	issuer   string
	audience string
}

func NewSigner(key ed25519.PrivateKey, keyID, issuer, audience string) (*Signer, error) {
	if len(key) != ed25519.PrivateKeySize || keyID == "" || issuer == "" || audience == "" {
		return nil, errors.New("invalid token signer configuration")
	}
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.EdDSA,
		Key:       jose.JSONWebKey{Key: key, KeyID: keyID, Algorithm: string(jose.EdDSA), Use: "sig"},
	}, (&jose.SignerOptions{}).WithType("at+jwt"))
	if err != nil {
		return nil, errors.New("invalid token signer configuration")
	}
	return &Signer{signer: signer, issuer: issuer, audience: audience}, nil
}

func (s *Signer) Sign(claims app.AccessTokenClaims) (string, error) {
	if claims.Subject == "" || claims.SessionID == "" || claims.ID == "" || !claims.ExpiresAt.After(claims.IssuedAt) {
		return "", errors.New("invalid access token claims")
	}
	return jwt.Signed(s.signer).Claims(jwt.Claims{
		Issuer: s.issuer, Subject: claims.Subject, Audience: jwt.Audience{s.audience},
		IssuedAt: jwt.NewNumericDate(claims.IssuedAt),
		Expiry:   jwt.NewNumericDate(claims.ExpiresAt), ID: claims.ID,
	}).Claims(struct {
		SessionID string `json:"sid"`
	}{SessionID: claims.SessionID}).Serialize()
}
