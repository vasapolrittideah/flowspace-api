package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

func TestTokenVerifierRequiresLiveVerifiedSession(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	jwksState := new(atomic.Int32)
	server := httptest.NewServer(testJWKSHandler(jwksState, public))
	defer server.Close()
	issuer, audience := "urn:flowspace:identity:test", "flowspace-api"
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: jose.JSONWebKey{Key: private, KeyID: "key-1"}}, (&jose.SignerOptions{}).WithType("at+jwt"))
	if err != nil {
		t.Fatal(err)
	}
	accessToken, err := jwt.Signed(signer).Claims(jwt.Claims{
		Issuer: issuer, Subject: "subject-1", Audience: jwt.Audience{audience},
		IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)), Expiry: jwt.NewNumericDate(time.Now().Add(time.Minute)), ID: "token-1",
	}).Claims(struct {
		SessionID string `json:"sid"`
	}{SessionID: "session-1"}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	checks := 0
	verified := false
	verifier := &TokenVerifier{jwksURL: server.URL, issuer: issuer, audience: audience, client: server.Client(), check: func(_ context.Context, request *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
		checks++
		if request.GetSubject() != "subject-1" || request.GetSessionId() != "session-1" {
			t.Fatalf("check request = %+v", request)
		}
		return &identityv1.CheckSessionResponse{EmailVerified: verified}, nil
	}}
	if _, err := verifier.VerifyToken(t.Context(), accessToken); !errors.Is(err, outbound.ErrEmailUnverified) || checks != 1 {
		t.Fatalf("unverified: error = %v, checks = %d", err, checks)
	}
	verified = true
	if subject, err := verifier.VerifyToken(t.Context(), accessToken); err != nil || subject != "subject-1" || checks != 2 {
		t.Fatalf("verified: subject = %q, error = %v, checks = %d", subject, err, checks)
	}
	verifier.check = func(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
		return nil, status.Error(codes.Unauthenticated, "inactive")
	}
	if subject, err := verifier.VerifyToken(t.Context(), accessToken); subject != "" || !errors.Is(err, outbound.ErrUnauthenticated) {
		t.Fatalf("revoked: subject = %q, error = %v", subject, err)
	}
	verifier.check = func(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
		return nil, status.Error(codes.Unavailable, "offline")
	}
	if subject, err := verifier.VerifyToken(t.Context(), accessToken); subject != "" || !errors.Is(err, outbound.ErrIdentityUnavailable) {
		t.Fatalf("outage: subject = %q, error = %v", subject, err)
	}
	assertJWKSFailures(t, verifier, accessToken, jwksState)
}

func testJWKSHandler(state *atomic.Int32, public ed25519.PublicKey) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch state.Load() {
		case 1:
			http.Error(w, "offline", http.StatusServiceUnavailable)
		case 2:
			_, _ = w.Write([]byte("invalid JSON"))
		case 3:
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{})
		default:
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: public, KeyID: "key-1", Algorithm: "EdDSA", Use: "sig"}}})
		}
	})
}

func assertJWKSFailures(t *testing.T, verifier *TokenVerifier, accessToken string, jwksState *atomic.Int32) {
	t.Helper()
	for _, state := range []int32{1, 2, 3} {
		jwksState.Store(state)
		_, err := verifier.VerifyToken(t.Context(), accessToken)
		want := outbound.ErrIdentityUnavailable
		if state == 3 {
			want = outbound.ErrUnauthenticated
		}
		if !errors.Is(err, want) {
			t.Fatalf("JWKS state %d error = %v, want %v", state, err, want)
		}
	}
}
