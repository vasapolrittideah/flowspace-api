package token

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func TestAccessTokenVerifier(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(public, "local-1", "issuer", "audience")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner(private, "local-1", "issuer", "audience")
	if err != nil {
		t.Fatal(err)
	}
	good, err := signer.Sign(outbound.AccessTokenClaims{
		Subject: "subject", SessionID: "session", ID: "token-id", IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := verifier.Verify(good); err != nil || got.Subject != "subject" || got.SessionID != "session" {
		t.Fatalf("verified identity = %+v, %v", got, err)
	}
	_, otherPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherSigner, err := NewSigner(otherPrivate, "local-1", "issuer", "audience")
	if err != nil {
		t.Fatal(err)
	}
	badSignature, err := otherSigner.Sign(outbound.AccessTokenClaims{
		Subject: "subject", SessionID: "session", ID: "token-id", IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		token string
	}{
		{"missing", ""}, {"malformed", "not-a-jwt"}, {"wrong signature", badSignature},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := verifier.Verify(test.token); !errors.Is(err, outbound.ErrUnauthenticated) {
				t.Fatalf("verification error = %v", err)
			}
		})
	}
}

func TestAccessTokenVerifierKeyOverlapAndRetirement(t *testing.T) {
	oldPublic, oldPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	newPublic, newPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	claims := outbound.AccessTokenClaims{
		Subject: "subject", SessionID: "session", ID: "token-id",
		IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Minute),
	}
	for _, candidate := range []struct {
		id  string
		key ed25519.PrivateKey
	}{
		{"old", oldPrivate}, {"new", newPrivate},
	} {
		signer, err := NewSigner(candidate.key, candidate.id, "issuer", "audience")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := signer.Sign(claims)
		if err != nil {
			t.Fatal(err)
		}
		verifier, err := NewVerifierKeys(map[string]ed25519.PublicKey{"old": oldPublic, "new": newPublic}, "issuer", "audience")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := verifier.Verify(raw); err != nil {
			t.Fatalf("overlap rejected %s token: %v", candidate.id, err)
		}
		if candidate.id == "old" {
			retired, err := NewVerifierKeys(map[string]ed25519.PublicKey{"new": newPublic}, "issuer", "audience")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := retired.Verify(raw); !errors.Is(err, outbound.ErrUnauthenticated) {
				t.Fatalf("retired key error = %v", err)
			}
		}
	}
}

func TestAccessTokenVerifierRejectsInvalidClaims(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(public, "local-1", "issuer", "audience")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, typ, keyID, issuer, audience, subject, sessionID string
		expires                                                time.Time
	}{
		{"wrong type", "JWT", "local-1", "issuer", "audience", "subject", "session", time.Now().Add(time.Minute)},
		{"wrong key", "at+jwt", "other-key", "issuer", "audience", "subject", "session", time.Now().Add(time.Minute)},
		{"wrong issuer", "at+jwt", "local-1", "other", "audience", "subject", "session", time.Now().Add(time.Minute)},
		{"wrong audience", "at+jwt", "local-1", "issuer", "other", "subject", "session", time.Now().Add(time.Minute)},
		{"expired", "at+jwt", "local-1", "issuer", "audience", "subject", "session", time.Now().Add(-time.Minute)},
		{"missing subject", "at+jwt", "local-1", "issuer", "audience", "", "session", time.Now().Add(time.Minute)},
		{"missing session", "at+jwt", "local-1", "issuer", "audience", "subject", "", time.Now().Add(time.Minute)},
		{"missing expiry", "at+jwt", "local-1", "issuer", "audience", "subject", "session", time.Time{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			joseSigner, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: jose.JSONWebKey{Key: private, KeyID: test.keyID}},
				(&jose.SignerOptions{}).WithType(jose.ContentType(test.typ)))
			if err != nil {
				t.Fatal(err)
			}
			claims := jwt.Claims{
				Issuer: test.issuer, Subject: test.subject, Audience: jwt.Audience{test.audience},
				IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)), ID: "token-id",
			}
			if !test.expires.IsZero() {
				claims.Expiry = jwt.NewNumericDate(test.expires)
			}
			raw, err := jwt.Signed(joseSigner).Claims(claims).Claims(struct {
				SessionID string `json:"sid"`
			}{SessionID: test.sessionID}).Serialize()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := verifier.Verify(raw); !errors.Is(err, outbound.ErrUnauthenticated) {
				t.Fatalf("verification error = %v", err)
			}
		})
	}
}
