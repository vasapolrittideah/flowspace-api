package keycloak

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"

	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

type TokenVerifier struct {
	verify func(context.Context, string) (*oidc.IDToken, error)
}

var _ outbound.TokenVerifier = (*TokenVerifier)(nil)

func NewTokenVerifier(ctx context.Context, issuer, audience string) (*TokenVerifier, error) {
	if issuer == "" || audience == "" {
		return nil, errors.New("OIDC issuer and audience are required")
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	return &TokenVerifier{verify: provider.Verifier(&oidc.Config{ClientID: audience}).Verify}, nil
}

func (v *TokenVerifier) VerifyToken(ctx context.Context, rawToken string) (string, error) {
	token, err := v.verify(ctx, rawToken)
	if err != nil {
		return "", fmt.Errorf("verify token: %w", err)
	}
	if token.Subject == "" {
		return "", errors.New("verified token has no subject")
	}
	return token.Subject, nil
}
