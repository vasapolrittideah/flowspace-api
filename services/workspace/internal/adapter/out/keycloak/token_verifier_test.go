package keycloak

import (
	"context"
	"errors"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
)

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
