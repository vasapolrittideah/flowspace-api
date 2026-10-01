package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/internal/requestid"
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

func TestPasswordLoginTelemetryOmitsCredentials(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}), zap.New(core))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/v1/password-sessions?email=User@example.com", strings.NewReader("secret-password"))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || logs.Len() != 1 {
		t.Fatalf("login log count = %d, status = %d", logs.Len(), response.Code)
	}
	fields := fmt.Sprint(logs.All()[0].ContextMap())
	if strings.Contains(fields, "User@example.com") || strings.Contains(fields, "secret-password") ||
		logs.All()[0].ContextMap()["operation"] != "POST /v1/password-sessions" {
		t.Fatal("login telemetry contains request data or misses its operation")
	}
}

func TestRefreshTelemetryOmitsToken(t *testing.T) {
	for _, outcome := range []int{http.StatusOK, http.StatusUnauthorized} {
		core, logs := observer.New(zap.InfoLevel)
		handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(outcome)
		}), zap.New(core))
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			"/v1/session-refreshes?refresh_token=secret-refresh-token", strings.NewReader(`{"refreshToken":"secret-refresh-token"}`))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != outcome || logs.Len() != 1 {
			t.Fatalf("refresh log count = %d, status = %d", logs.Len(), response.Code)
		}
		fields := fmt.Sprint(logs.All()[0].ContextMap())
		if strings.Contains(fields, "secret-refresh-token") || logs.All()[0].ContextMap()["operation"] != "POST /v1/session-refreshes" {
			t.Fatal("refresh telemetry contains token or misses its operation")
		}
	}
}

func TestLogoutTelemetryOmitsToken(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), zap.New(core))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/session-logouts?access_token=secret-access-token", nil)
	request.Header.Set("Authorization", "Bearer secret-access-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || logs.Len() != 1 {
		t.Fatalf("logout log count = %d, status = %d", logs.Len(), response.Code)
	}
	fields := fmt.Sprint(logs.All()[0].ContextMap())
	if strings.Contains(fields, "secret-access-token") || logs.All()[0].ContextMap()["operation"] != "POST /v1/session-logouts" {
		t.Fatal("logout telemetry contains token or misses its operation")
	}
}

func TestAllLogoutTelemetryOmitsToken(t *testing.T) {
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/account-session-logouts?access_token=secret-access-token", nil)
	request.Header.Set("Authorization", "Bearer secret-access-token")
	operation := safeOperation(request)
	if operation != "POST /v1/account-session-logouts" || strings.Contains(operation, "secret-access-token") {
		t.Fatal("all-session logout telemetry contains token or misses its operation")
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

func (stubIdentityHandler) RequestPasswordResetCode(context.Context, *identityv1.RequestPasswordResetCodeRequest) (*identityv1.RequestPasswordResetCodeResponse, error) {
	return &identityv1.RequestPasswordResetCodeResponse{Accepted: true}, nil
}

func (stubIdentityHandler) ResetPassword(context.Context, *identityv1.ResetPasswordRequest) (*identityv1.ResetPasswordResponse, error) {
	return &identityv1.ResetPasswordResponse{PasswordChanged: true, SessionsRevoked: true}, nil
}

func (stubIdentityHandler) CheckSession(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
	return &identityv1.CheckSessionResponse{EmailVerified: true}, nil
}

func TestPublicHandlerOnlyServesApprovedRoutes(t *testing.T) {
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, http.NotFoundHandler(), zap.NewNop(), nil)
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
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, http.NotFoundHandler(), zap.NewNop(), nil)
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

func TestPublicHandlerServesPasswordRecoveryTypedRPC(t *testing.T) {
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, http.NotFoundHandler(), zap.NewNop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method  string
		message proto.Message
		length  byte
	}{
		{"RequestPasswordResetCode", &identityv1.RequestPasswordResetCodeRequest{Email: "a@example.com"}, 15},
		{"ResetPassword", &identityv1.ResetPasswordRequest{Email: "a@example.com", Code: "012345", NewPassword: "fresh password 123"}, 43},
	} {
		method := test.method
		payload, err := proto.Marshal(test.message)
		if err != nil {
			t.Fatal(err)
		}
		if len(payload) != int(test.length) {
			t.Fatalf("unexpected %s test payload size: %d", method, len(payload))
		}
		frame := append([]byte{0, 0, 0, 0, test.length}, payload...)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			"/flowspace.identity.v1.IdentityService/"+method, strings.NewReader(string(frame)))
		request.ProtoMajor = 2
		request.Header.Set("Content-Type", "application/grpc")
		request.Header.Set("TE", "trailers")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Result().Trailer.Get("Grpc-Status") != "0" {
			t.Fatalf("%s typed RPC status = %d, trailer = %v", method, response.Code, response.Result().Trailer)
		}
	}
}

func TestPasswordRecoveryTelemetryOmitsSecrets(t *testing.T) {
	for _, test := range []struct{ path, body string }{
		{"/v1/password-reset-codes", `{"email":"User@example.com"}`},
		{"/v1/password-resets", `{"email":"User@example.com","code":"012345","newPassword":"secret-new-password"}`},
	} {
		path, body := test.path, test.body
		for _, outcome := range []int{http.StatusOK, http.StatusBadRequest, http.StatusTooManyRequests} {
			core, logs := observer.New(zap.InfoLevel)
			handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(outcome)
			}), zap.New(core))
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
				path+"?email=User@example.com&code=012345", strings.NewReader(body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != outcome || logs.Len() != 1 {
				t.Fatalf("%s log count = %d, status = %d", path, logs.Len(), response.Code)
			}
			fields := fmt.Sprint(logs.All()[0].ContextMap())
			if strings.Contains(fields, "User@example.com") || strings.Contains(fields, "012345") ||
				strings.Contains(fields, "secret-new-password") || logs.All()[0].ContextMap()["operation"] != "POST "+path {
				t.Fatalf("%s telemetry contains request data or misses its operation: %s", path, fields)
			}
		}
	}
}

func TestProviderLoginTelemetryOmitsSecrets(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"authorizationUrl":"https://accounts.google.com/o/oauth2/v2/auth?state=secret-state","attemptToken":"secret-attempt"}`))
	}), zap.New(core))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/v1/provider-login-attempts?state=secret-state", strings.NewReader(`{"provider":"PROVIDER_GOOGLE"}`))
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if logs.Len() != 1 {
		t.Fatalf("provider login log count = %d", logs.Len())
	}
	fields := fmt.Sprint(logs.All()[0].ContextMap())
	if strings.Contains(fields, "secret-") || logs.All()[0].ContextMap()["operation"] != "POST /v1/provider-login-attempts" {
		t.Fatalf("provider login telemetry contains secrets or misses its operation: %s", fields)
	}
}

func TestPublicHandlerRoutesProviderCallbacks(t *testing.T) {
	var provider string
	callback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider = r.PathValue("provider")
		w.WriteHeader(http.StatusTeapot)
	})
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, callback, zap.NewNop(), nil)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/provider-login-callbacks/google?state=s&code=c", nil))
	if response.Code != http.StatusTeapot || provider != "google" {
		t.Fatalf("callback route status = %d, provider = %q", response.Code, provider)
	}
	provider = ""
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/provider-login-callbacks/google", nil))
	if response.Code != http.StatusNotFound || provider != "" {
		t.Fatalf("callback POST status = %d", response.Code)
	}
}

func TestProviderCallbackTelemetryOmitsSecrets(t *testing.T) {
	for _, path := range []string{"/v1/provider-login-callbacks/google", "/v1/provider-login-callbacks/github"} {
		core, logs := observer.New(zap.InfoLevel)
		handler := observeRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("secret-handoff"))
		}), zap.New(core))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			path+"?state=secret-state&code=secret-code", nil))
		fields := fmt.Sprint(logs.All()[0].ContextMap())
		if logs.Len() != 1 || strings.Contains(fields, "secret-") || logs.All()[0].ContextMap()["operation"] != "GET "+path {
			t.Fatalf("callback telemetry contains secrets or misses its operation: %s", fields)
		}
	}
}

func TestPublicHandlerRejectsCheckSession(t *testing.T) {
	handler, err := newPublicHandler(t.Context(), stubIdentityHandler{}, http.NotFoundHandler(), zap.NewNop(), nil)
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
	if requestid.Valid("bad value") || requestid.Valid(strings.Repeat("a", 129)) || !requestid.Valid("safe-1") {
		t.Fatal("request ID validation failed")
	}
	if _, err := loadTrustedProxies("0.0.0.0/0"); err == nil {
		t.Fatal("accepted wildcard trusted proxy")
	}
	if prefixes, err := loadTrustedProxies("127.0.0.1/32"); err != nil || len(prefixes) != 1 {
		t.Fatalf("trusted proxies = %v, %v", prefixes, err)
	}
}
