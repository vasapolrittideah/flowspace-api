package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionHandlerPreventsSharedCaching(t *testing.T) {
	handler := NewSessionHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"secret"}`))
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/accounts", nil))
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control = %q", response.Header().Get("Cache-Control"))
	}
}
