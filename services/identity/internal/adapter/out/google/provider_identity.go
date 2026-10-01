package google

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

// Endpoints are the Google token, signing-key, and ID token issuer locations.
type Endpoints struct {
	CodeExchange string
	Keys         string
	Issuer       string
}

// DefaultEndpoints are Google's published OpenID Connect endpoints.
var DefaultEndpoints = Endpoints{
	CodeExchange: "https://oauth2.googleapis.com/token",
	Keys:         "https://www.googleapis.com/oauth2/v3/certs",
	Issuer:       "https://accounts.google.com",
}

// ProviderIdentity exchanges a Google authorization code and verifies its ID token.
type ProviderIdentity struct {
	clientID, clientSecret string
	tokenURL               string
	client                 *http.Client
	verifier               *oidc.IDTokenVerifier
}

var _ outbound.ProviderIdentityVerifier = (*ProviderIdentity)(nil)

// NewProviderIdentity fetches signing keys with ctx, so ctx must live as long as the verifier.
func NewProviderIdentity(ctx context.Context, clientID, clientSecret string, endpoints Endpoints, client *http.Client) *ProviderIdentity {
	keys := oidc.NewRemoteKeySet(oidc.ClientContext(ctx, client), endpoints.Keys)
	return &ProviderIdentity{
		clientID: clientID, clientSecret: clientSecret, tokenURL: endpoints.CodeExchange, client: client,
		// The verifier checks the signature, issuer, audience, and expiry.
		verifier: oidc.NewVerifier(endpoints.Issuer, keys, &oidc.Config{ClientID: clientID}),
	}
}

func (p *ProviderIdentity) VerifyProviderIdentity(ctx context.Context, exchange outbound.ProviderCodeExchange) (outbound.ProviderIdentity, error) {
	config := oauth2.Config{
		ClientID: p.clientID, ClientSecret: p.clientSecret, RedirectURL: exchange.CallbackURL,
		Endpoint: oauth2.Endpoint{TokenURL: p.tokenURL, AuthStyle: oauth2.AuthStyleInParams},
	}
	token, err := config.Exchange(oidc.ClientContext(ctx, p.client), exchange.Code, oauth2.VerifierOption(exchange.CodeVerifier))
	if err != nil {
		var rejected *oauth2.RetrieveError
		if errors.As(err, &rejected) && rejected.Response != nil && rejected.Response.StatusCode < http.StatusInternalServerError {
			return outbound.ProviderIdentity{}, domain.ErrInvalidProviderProof
		}
		return outbound.ProviderIdentity{}, errors.Join(domain.ErrProviderUnavailable, err)
	}
	raw, _ := token.Extra("id_token").(string)
	if raw == "" {
		return outbound.ProviderIdentity{}, domain.ErrInvalidProviderProof
	}
	// ponytail: a failed signing-key fetch is reported as invalid proof, not as an outage.
	idToken, err := p.verifier.Verify(ctx, raw)
	if err != nil {
		if ctx.Err() != nil {
			return outbound.ProviderIdentity{}, errors.Join(domain.ErrProviderUnavailable, ctx.Err())
		}
		return outbound.ProviderIdentity{}, domain.ErrInvalidProviderProof
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil || idToken.Subject == "" || exchange.Nonce == "" ||
		subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(exchange.Nonce)) != 1 {
		return outbound.ProviderIdentity{}, domain.ErrInvalidProviderProof
	}
	// A missing or non-boolean email_verified claim is not verified-email proof.
	email, _ := claims["email"].(string)
	verified, _ := claims["email_verified"].(bool)
	hostedDomain, _ := claims["hd"].(string)
	return outbound.ProviderIdentity{Subject: idToken.Subject, Email: email, EmailVerified: verified, HostedDomain: hostedDomain}, nil
}
