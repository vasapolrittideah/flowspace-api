package http

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/netip"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

var callbackPage = template.Must(template.New("callback").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>FlowSpace sign-in</title></head>
<body>
<main>
{{if .Code}}<h1>Copy your sign-in code</h1>
<p>Paste this code into your FlowSpace client. It works once and expires in ten minutes.</p>
<p><code>{{.Code}}</code></p>
{{else}}<h1>Unable to complete sign-in</h1>
<p>{{.Message}}</p>
{{end}}</main>
</body>
</html>
`))

// ProviderCallbackHandler serves the fixed provider callback routes. It passes
// the provider response to Identity and shows only the one-time handoff code.
type ProviderCallbackHandler struct {
	service inbound.ProviderLoginService
	trusted []netip.Prefix
}

func NewProviderCallbackHandler(service inbound.ProviderLoginService, trusted []netip.Prefix) *ProviderCallbackHandler {
	return &ProviderCallbackHandler{service: service, trusted: trusted}
}

func (h *ProviderCallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	headers := w.Header()
	headers.Set("Cache-Control", "no-store")
	headers.Set("Referrer-Policy", "no-referrer")
	headers.Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	headers.Set("X-Content-Type-Options", "nosniff")
	headers.Set("Content-Type", "text/html; charset=utf-8")
	source, err := SourceAddress(r.RemoteAddr, r.Header, h.trusted)
	if err != nil {
		renderCallback(w, http.StatusBadRequest, "", "The request source is invalid.")
		return
	}
	if h.service == nil {
		renderCallback(w, http.StatusServiceUnavailable, "", "Sign-in is unavailable. Try again later.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	query := r.URL.Query()
	code, err := h.service.CompleteProviderCallback(ctx, inbound.CompleteProviderCallbackInput{
		Provider: r.PathValue("provider"), State: query.Get("state"), Code: query.Get("code"),
		Denied: query.Has("error"), Source: source,
	})
	switch {
	case err == nil:
		renderCallback(w, http.StatusOK, code, "")
	case errors.Is(err, app.ErrInvalidProviderCallback):
		renderCallback(w, http.StatusBadRequest, "", "This sign-in link is invalid, expired, or already used. Start a new sign-in from your FlowSpace client.")
	case errors.Is(err, app.ErrRateLimited):
		renderCallback(w, http.StatusTooManyRequests, "", "Too many sign-in attempts. Try again later.")
	default:
		renderCallback(w, http.StatusServiceUnavailable, "", "Sign-in is unavailable. Try again later.")
	}
}

func renderCallback(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	_ = callbackPage.Execute(w, struct{ Code, Message string }{code, message})
}
