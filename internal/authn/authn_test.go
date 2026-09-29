package authn

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
)

func TestVerifyAccessTokenReturnsValidatedClaims(t *testing.T) {
	raw, public := signedTestAccessToken(t, "key-1")
	keyID := ""
	claims, err := VerifyAccessToken(raw, "issuer", "audience", func(id string) (ed25519.PublicKey, error) {
		keyID = id
		return public, nil
	})
	if err != nil || claims != (Claims{Subject: "subject-1", SessionID: "session-1"}) || keyID != "key-1" {
		t.Fatalf("claims = %+v, key ID = %q, error = %v", claims, keyID, err)
	}
}

func TestVerifyAccessTokenRejectsMissingKeyID(t *testing.T) {
	raw, public := signedTestAccessToken(t, "")
	keyRequested := false
	claims, err := VerifyAccessToken(raw, "issuer", "audience", func(string) (ed25519.PublicKey, error) {
		keyRequested = true
		return public, nil
	})
	if claims != (Claims{}) || !errors.Is(err, ErrUnauthenticated) || keyRequested {
		t.Fatalf("claims = %+v, key requested = %t, error = %v", claims, keyRequested, err)
	}
}

func TestVerifyAccessTokenReportsKeyLookupFailure(t *testing.T) {
	raw, _ := signedTestAccessToken(t, "key-1")
	claims, err := VerifyAccessToken(raw, "issuer", "audience", func(string) (ed25519.PublicKey, error) {
		return nil, ErrUnavailable
	})
	if claims != (Claims{}) || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("claims = %+v, error = %v", claims, err)
	}
}

func TestVerifyAccessTokenRejectsInvalidToken(t *testing.T) {
	raw, public := signedTestAccessToken(t, "key-1")
	otherPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, raw, issuer, audience string
		key                         ed25519.PublicKey
	}{
		{name: "malformed", raw: "not-a-jwt", issuer: "issuer", audience: "audience", key: public},
		{name: "wrong signature", raw: raw, issuer: "issuer", audience: "audience", key: otherPublic},
		{name: "wrong issuer", raw: raw, issuer: "other", audience: "audience", key: public},
		{name: "wrong audience", raw: raw, issuer: "issuer", audience: "other", key: public},
	} {
		t.Run(test.name, func(t *testing.T) {
			claims, err := VerifyAccessToken(test.raw, test.issuer, test.audience, func(string) (ed25519.PublicKey, error) {
				return test.key, nil
			})
			if claims != (Claims{}) || !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("claims = %+v, error = %v", claims, err)
			}
		})
	}
}

func signedTestAccessToken(t *testing.T, keyID string) (string, ed25519.PublicKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: jose.JSONWebKey{Key: private, KeyID: keyID}}, (&jose.SignerOptions{}).WithType("at+jwt"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).Claims(jwt.Claims{
		Issuer: "issuer", Subject: "subject-1", Audience: jwt.Audience{"audience"},
		IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)), Expiry: jwt.NewNumericDate(time.Now().Add(time.Minute)), ID: "token-1",
	}).Claims(struct {
		SessionID string `json:"sid"`
	}{SessionID: "session-1"}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw, public
}

func TestCheckSessionRejectsIncompleteClaims(t *testing.T) {
	for _, test := range []struct {
		name   string
		claims Claims
	}{
		{name: "missing subject", claims: Claims{SessionID: "session-1"}},
		{name: "missing session ID", claims: Claims{Subject: "subject-1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			verified, err := CheckSession(t.Context(), test.claims, func(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
				called = true
				return &identityv1.CheckSessionResponse{EmailVerified: true}, nil
			})
			if verified || !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("verified = %t, error = %v", verified, err)
			}
			if called {
				t.Fatal("session check called with incomplete claims")
			}
		})
	}
}

func TestCheckSessionUsesCurrentResult(t *testing.T) {
	claims := Claims{Subject: "subject-1", SessionID: "session-1"}
	checks := 0
	emailVerified := false
	check := func(_ context.Context, request *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
		checks++
		if request.GetSubject() != claims.Subject || request.GetSessionId() != claims.SessionID {
			t.Fatalf("request = %+v", request)
		}
		return &identityv1.CheckSessionResponse{EmailVerified: emailVerified}, nil
	}
	verified, err := CheckSession(t.Context(), claims, check)
	if verified || err != nil || checks != 1 {
		t.Fatalf("first check: verified = %t, error = %v, checks = %d", verified, err, checks)
	}
	emailVerified = true
	verified, err = CheckSession(t.Context(), claims, check)
	if !verified || err != nil || checks != 2 {
		t.Fatalf("second check: verified = %t, error = %v, checks = %d", verified, err, checks)
	}
}

func TestCheckSessionFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name     string
		checkErr error
		want     error
	}{
		{name: "revoked", checkErr: status.Error(codes.Unauthenticated, "revoked"), want: ErrUnauthenticated},
		{name: "unavailable", checkErr: status.Error(codes.Unavailable, "offline"), want: ErrUnavailable},
		{name: "missing response", want: ErrUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			verified, err := CheckSession(t.Context(), Claims{Subject: "subject-1", SessionID: "session-1"}, func(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
				return nil, test.checkErr
			})
			if verified || !errors.Is(err, test.want) {
				t.Fatalf("verified = %t, error = %v, want %v", verified, err, test.want)
			}
		})
	}
}
