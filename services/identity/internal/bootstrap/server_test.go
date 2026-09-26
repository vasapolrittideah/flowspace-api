package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
)

func TestSigningKeyPublicationBeforeAndAfterSwitch(t *testing.T) {
	oldPublic, oldPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	newPublic, newPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []struct {
		name, activeID, additional string
		active                     ed25519.PrivateKey
		want                       map[string]ed25519.PublicKey
	}{
		{
			"published", "old", "new:" + base64.RawURLEncoding.EncodeToString(newPublic), oldPrivate,
			map[string]ed25519.PublicKey{"old": oldPublic, "new": newPublic},
		},
		{
			"switched", "new", "old:" + base64.RawURLEncoding.EncodeToString(oldPublic), newPrivate,
			map[string]ed25519.PublicKey{"old": oldPublic, "new": newPublic},
		},
		{"retired", "new", "", newPrivate, map[string]ed25519.PublicKey{"new": newPublic}},
	} {
		t.Run(phase.name, func(t *testing.T) {
			keys, document, err := buildSigningKeys(phase.active, phase.activeID, phase.additional)
			if err != nil {
				t.Fatal(err)
			}
			var jwks jose.JSONWebKeySet
			if err := json.Unmarshal(document, &jwks); err != nil {
				t.Fatal(err)
			}
			if len(keys) != len(phase.want) || len(jwks.Keys) != len(phase.want) {
				t.Fatalf("published %d verifier keys and %d JWKS keys, want %d", len(keys), len(jwks.Keys), len(phase.want))
			}
			for _, key := range jwks.Keys {
				want, ok := phase.want[key.KeyID]
				published, valid := key.Key.(ed25519.PublicKey)
				if !ok || !valid || string(keys[key.KeyID]) != string(want) || string(published) != string(want) {
					t.Fatalf("wrong public key for %q", key.KeyID)
				}
			}
		})
	}
}

func TestSigningKeyPublicationRejectsInvalidAdditionalKey(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"current:AA", "current:" + base64.RawURLEncoding.EncodeToString(publicKey), "missing-colon"} {
		if _, _, err := buildSigningKeys(privateKey, "current", value); err == nil {
			t.Fatalf("accepted invalid additional key %q", value)
		}
	}
}

type stubIdentityHandler struct {
	identityv1.UnimplementedIdentityServiceServer
}

func (stubIdentityHandler) CreateAccount(context.Context, *identityv1.CreateAccountRequest) (*identityv1.CreateAccountResponse, error) {
	return &identityv1.CreateAccountResponse{Subject: "subject-1"}, nil
}

func (stubIdentityHandler) CheckSession(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
	return &identityv1.CheckSessionResponse{EmailVerified: true}, nil
}

func TestPublicHandlerOnlyServesApprovedRoutes(t *testing.T) {
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, zap.NewNop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/accounts", strings.NewReader(`{"email":"a@example.com","password":"password"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "request-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("X-Request-ID") != "request-1" {
		t.Fatalf("approved route: status %d, request ID %q", response.Code, response.Header().Get("X-Request-ID"))
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/.well-known/jwks.json", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("public JWKS status = %d", response.Code)
	}
}

func TestPublicHandlerServesTypedRPC(t *testing.T) {
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, zap.NewNop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := proto.Marshal(&identityv1.CreateAccountRequest{Email: "a@example.com", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	const payloadLength = 25
	if len(payload) != payloadLength {
		t.Fatalf("unexpected test payload size: %d", len(payload))
	}
	frame := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(frame[1:5], payloadLength)
	copy(frame[5:], payload)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/flowspace.identity.v1.IdentityService/CreateAccount", strings.NewReader(string(frame)))
	request.ProtoMajor = 2
	request.Header.Set("Content-Type", "application/grpc")
	request.Header.Set("TE", "trailers")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Result().Trailer.Get("Grpc-Status") != "0" {
		t.Fatalf("typed RPC status = %d, trailer = %v", response.Code, response.Result().Trailer)
	}
}

func TestPublicHandlerRejectsCheckSession(t *testing.T) {
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, zap.NewNop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := proto.Marshal(&identityv1.CheckSessionRequest{Subject: "subject-1", SessionId: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	const payloadLength = 22
	if len(payload) != payloadLength {
		t.Fatalf("unexpected test payload size: %d", len(payload))
	}
	frame := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(frame[1:5], payloadLength)
	copy(frame[5:], payload)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/flowspace.identity.v1.IdentityService/CheckSession", strings.NewReader(string(frame)))
	request.ProtoMajor = 2
	request.Header.Set("Content-Type", "application/grpc")
	request.Header.Set("TE", "trailers")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Result().Trailer.Get("Grpc-Status"); got != "12" {
		t.Fatalf("public CheckSession gRPC status = %q, want 12", got)
	}
}

func TestPublicRequestIdentifiersAndProxyConfiguration(t *testing.T) {
	if validRequestID("bad value") || validRequestID(strings.Repeat("a", 129)) || !validRequestID("safe-1") {
		t.Fatal("request ID validation failed")
	}
	if _, err := loadTrustedProxies("0.0.0.0/0"); err == nil {
		t.Fatal("accepted wildcard trusted proxy")
	}
	if prefixes, err := loadTrustedProxies("127.0.0.1/32"); err != nil || len(prefixes) != 1 {
		t.Fatalf("trusted proxies = %v, %v", prefixes, err)
	}
}
