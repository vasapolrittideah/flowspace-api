package bootstrap

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
)

type stubIdentityHandler struct {
	identityv1.UnimplementedIdentityServiceServer
}

func (stubIdentityHandler) CreateAccount(context.Context, *identityv1.CreateAccountRequest) (*identityv1.CreateAccountResponse, error) {
	return &identityv1.CreateAccountResponse{Subject: "subject-1"}, nil
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
