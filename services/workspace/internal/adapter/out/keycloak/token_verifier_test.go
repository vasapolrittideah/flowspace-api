package keycloak

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
)

const (
	testIssuer   = "https://identity.test/realms/flowspace"
	testAudience = "workspace-api"
	testKeyID    = "workspace-test-key"
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

func TestTokenVerifierValidatesOIDCTokens(t *testing.T) {
	fixture := newOIDCFixture(t)
	verifier, err := NewTokenVerifier(t.Context(), fixture.server.URL, testIssuer, testAudience)
	if err != nil {
		t.Fatal(err)
	}

	validClaims := fixture.claims()
	validClaims["email"] = "untrusted@example.com"
	validClaims["workspace_id"] = "untrusted-workspace"
	validClaims["roles"] = []string{"owner"}
	subject, err := verifier.VerifyToken(t.Context(), fixture.sign(t, fixture.privateKey, validClaims))
	if err != nil {
		t.Fatal(err)
	}
	if subject != "subject-1" {
		t.Fatalf("subject = %q, want subject-1", subject)
	}

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		key    *rsa.PrivateKey
		change func(map[string]any)
	}{
		{
			name: "wrong signature",
			key:  otherKey,
		},
		{
			name: "wrong issuer",
			change: func(claims map[string]any) {
				claims["iss"] = fixture.server.URL
			},
		},
		{
			name: "wrong audience",
			change: func(claims map[string]any) {
				claims["aud"] = "another-api"
			},
		},
		{
			name: "expired",
			change: func(claims map[string]any) {
				claims["exp"] = time.Now().Add(-time.Minute).Unix()
			},
		},
		{
			name: "missing subject",
			change: func(claims map[string]any) {
				delete(claims, "sub")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := fixture.claims()
			if tt.change != nil {
				tt.change(claims)
			}
			key := tt.key
			if key == nil {
				key = fixture.privateKey
			}
			if _, err := verifier.VerifyToken(t.Context(), fixture.sign(t, key, claims)); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
}

func TestTokenVerifierPreservesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	verifier := &TokenVerifier{verify: func(gotContext context.Context, _ string) (*oidc.IDToken, error) {
		if gotContext != ctx {
			t.Fatal("verify context differs from request context")
		}
		return nil, errors.New("verification stopped")
	}}

	_, err := verifier.VerifyToken(ctx, "token")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

type oidcFixture struct {
	privateKey *rsa.PrivateKey
	server     *httptest.Server
}

func newOIDCFixture(t *testing.T) *oidcFixture {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &oidcFixture{privateKey: privateKey}

	keyServer := &oidctest.Server{PublicKeys: []oidctest.PublicKey{{
		PublicKey: privateKey.Public(),
		KeyID:     testKeyID,
		Algorithm: oidc.RS256,
	}}}
	var serverURL string
	handler := http.NewServeMux()
	handler.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                testIssuer,
			"jwks_uri":                              serverURL + "/keys",
			"id_token_signing_alg_values_supported": []string{oidc.RS256},
		}); err != nil {
			t.Error(err)
		}
	})
	handler.HandleFunc("/keys", func(w http.ResponseWriter, request *http.Request) {
		keyServer.ServeHTTP(w, request)
	})
	fixture.server = httptest.NewServer(handler)
	serverURL = fixture.server.URL
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *oidcFixture) claims() map[string]any {
	return map[string]any{
		"iss": testIssuer,
		"aud": testAudience,
		"sub": "subject-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
}

func (f *oidcFixture) sign(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	rawClaims, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return oidctest.SignIDToken(privateKey, testKeyID, oidc.RS256, string(rawClaims))
}
