package keycloak

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
)

func TestNewTokenVerifierUsesSeparateDiscoveryURL(t *testing.T) {
	const issuer = "https://identity.test/realms/flowspace"
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body := fmt.Sprintf(`{"issuer":%q,"jwks_uri":%q,"id_token_signing_alg_values_supported":["RS256"]}`, issuer, "https://identity.test/keys")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
	ctx := oidc.ClientContext(context.Background(), client)

	if _, err := NewTokenVerifier(ctx, "http://keycloak/realms/flowspace", issuer, "workspace-api"); err != nil {
		t.Fatal(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestTokenVerifier(t *testing.T) {
	t.Run("returns the verified subject", func(t *testing.T) {
		verifier := &TokenVerifier{verify: func(context.Context, string) (*oidc.IDToken, error) {
			return &oidc.IDToken{Subject: "subject-1"}, nil
		}}

		subject, err := verifier.VerifyToken(context.Background(), "token")
		if err != nil {
			t.Fatal(err)
		}
		if subject != "subject-1" {
			t.Fatalf("subject = %q, want subject-1", subject)
		}
	})

	t.Run("rejects an empty subject", func(t *testing.T) {
		verifier := &TokenVerifier{verify: func(context.Context, string) (*oidc.IDToken, error) {
			return &oidc.IDToken{}, nil
		}}

		if _, err := verifier.VerifyToken(context.Background(), "token"); err == nil {
			t.Fatal("empty subject accepted")
		}
	})

	t.Run("rejects a failed verification", func(t *testing.T) {
		verifier := &TokenVerifier{verify: func(context.Context, string) (*oidc.IDToken, error) {
			return nil, errors.New("invalid token")
		}}

		if _, err := verifier.VerifyToken(context.Background(), "token"); err == nil {
			t.Fatal("verification error accepted")
		}
	})
}
