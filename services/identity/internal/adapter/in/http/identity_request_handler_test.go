package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
)

func TestSignupRequestHandlerRejectsIdempotencyKey(t *testing.T) {
	called := false
	handler := identityhttp.NewIdentityRequestHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), nil)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/accounts", nil)
	request.Header["Idempotency-Key"] = []string{""}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || called || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response status = %d, called = %t", response.Code, called)
	}
}

func TestSignupRequestHandlerValidatesSource(t *testing.T) {
	called := false
	handler := identityhttp.NewIdentityRequestHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	}), nil)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/accounts", nil)
	request.RemoteAddr = "not-an-address"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || called {
		t.Fatalf("invalid source status = %d, called = %t", response.Code, called)
	}
	request.RemoteAddr = "192.0.2.1:1234"
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !called {
		t.Fatalf("valid source status = %d, called = %t", response.Code, called)
	}
}
