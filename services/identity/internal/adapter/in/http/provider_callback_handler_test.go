package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

type fakeProviderCallback struct {
	fakeProviderLogin
	callback inbound.CompleteProviderCallbackInput
	code     string
	err      error
}

func (s *fakeProviderCallback) CompleteProviderCallback(_ context.Context, input inbound.CompleteProviderCallbackInput) (string, error) {
	s.callback = input
	return s.code, s.err
}

func serveProviderCallback(t *testing.T, service inbound.ProviderLoginService, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /v1/provider-login-callbacks/{provider}", identityhttp.NewProviderCallbackHandler(service, nil))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}

func checkCallbackPageHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	headers := response.Header()
	if headers.Get("Cache-Control") != "no-store" || headers.Get("Referrer-Policy") != "no-referrer" ||
		headers.Get("Content-Security-Policy") != "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'" ||
		headers.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(headers.Get("Content-Type"), "text/html") {
		t.Fatalf("callback headers = %v", headers)
	}
	body := response.Body.String()
	if strings.Contains(body, "http://") || strings.Contains(body, "https://") || strings.Contains(body, "<script") || strings.Contains(body, "<link") {
		t.Fatalf("callback page loads other resources: %s", body)
	}
}

func TestProviderCallbackShowsHandoffCodeOnce(t *testing.T) {
	service := &fakeProviderCallback{code: "handoff-code-value"}
	response := serveProviderCallback(t, service, "/v1/provider-login-callbacks/google?state=state-value&code=provider-code&scope=openid")
	checkCallbackPageHeaders(t, response)
	body := response.Body.String()
	want := inbound.CompleteProviderCallbackInput{Provider: "google", State: "state-value", Code: "provider-code", Source: "192.0.2.1"}
	if response.Code != http.StatusOK || service.callback != want || strings.Count(body, "handoff-code-value") != 1 {
		t.Fatalf("callback = %d %+v %s", response.Code, service.callback, body)
	}
	for _, secret := range []string{"state-value", "provider-code", "accessToken", "refreshToken"} {
		if strings.Contains(body, secret) {
			t.Fatalf("callback page contains %s", secret)
		}
	}
}

func TestProviderCallbackPassesProviderDenial(t *testing.T) {
	service := &fakeProviderCallback{err: app.ErrInvalidProviderCallback}
	response := serveProviderCallback(t, service, "/v1/provider-login-callbacks/github?state=state-value&error=access_denied")
	if !service.callback.Denied || service.callback.Provider != "github" || response.Code != http.StatusBadRequest {
		t.Fatalf("denial = %d %+v", response.Code, service.callback)
	}
}

func TestProviderCallbackFailuresAreSafe(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{app.ErrInvalidProviderCallback, http.StatusBadRequest},
		{app.ErrRateLimited, http.StatusTooManyRequests},
		{app.ErrLimitUnavailable, http.StatusServiceUnavailable},
		{app.ErrProviderLoginUnavailable, http.StatusServiceUnavailable},
		{context.DeadlineExceeded, http.StatusServiceUnavailable},
		{errors.New("database password=secret"), http.StatusServiceUnavailable},
	} {
		service := &fakeProviderCallback{code: "unused-code", err: test.err}
		response := serveProviderCallback(t, service, "/v1/provider-login-callbacks/google?state=state-value&code=provider-code")
		checkCallbackPageHeaders(t, response)
		body := response.Body.String()
		if response.Code != test.status || strings.Contains(body, "unused-code") || strings.Contains(body, "secret") || strings.Contains(body, "state-value") {
			t.Fatalf("error %v: status %d, body %s", test.err, response.Code, body)
		}
	}
}

func TestProviderCallbackRejectsInvalidSource(t *testing.T) {
	service := &fakeProviderCallback{}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/provider-login-callbacks/{provider}", identityhttp.NewProviderCallbackHandler(service, nil))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/provider-login-callbacks/google?state=s&code=c", nil)
	request.RemoteAddr = "not-an-address"
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || service.callback.State != "" {
		t.Fatalf("invalid source status = %d", response.Code)
	}
}
