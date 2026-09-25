package token

import (
	"crypto/ed25519"
	"crypto/rand"
	"reflect"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
)

func TestTokenSignerProducesEdDSAAccessJWT(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner(private, "local-1", "urn:flowspace:identity:local", "flowspace-api")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	access, err := signer.Sign(app.AccessTokenClaims{
		Subject: "subject-1", SessionID: "session-1", ID: "jti-1", IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.ParseSigned(access, []jose.SignatureAlgorithm{jose.EdDSA})
	if err != nil {
		t.Fatal(err)
	}
	if token.Headers[0].KeyID != "local-1" || token.Headers[0].Algorithm != string(jose.EdDSA) || token.Headers[0].ExtraHeaders[jose.HeaderKey("typ")] != "at+jwt" {
		t.Fatalf("wrong protected JWT header: %+v", token.Headers[0])
	}
	var standard jwt.Claims
	var extra struct {
		SessionID string `json:"sid"`
	}
	if err := token.Claims(public, &standard, &extra); err != nil {
		t.Fatal(err)
	}
	got := []any{standard.Issuer, standard.Subject, standard.Audience, standard.ID, extra.SessionID, standard.IssuedAt.Time().Unix(), standard.Expiry.Time().Unix()}
	want := []any{"urn:flowspace:identity:local", "subject-1", jwt.Audience{"flowspace-api"}, "jti-1", "session-1", now.Unix(), now.Add(10 * time.Minute).Unix()}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wrong JWT claims: %+v, %+v", standard, extra)
	}
}

func TestTokenSignerRejectsMissingKeyConfiguration(t *testing.T) {
	if _, err := NewSigner(nil, "local-1", "issuer", "audience"); err == nil {
		t.Fatal("accepted missing signing key")
	}
}
