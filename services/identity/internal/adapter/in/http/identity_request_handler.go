package http

import (
	"context"
	"net/http"
	"net/netip"
)

// IdentityRequestHandler supplies the connection source to the local REST gateway.
type IdentityRequestHandler struct {
	next    http.Handler
	trusted []netip.Prefix
}

func NewIdentityRequestHandler(next http.Handler, trusted []netip.Prefix) *IdentityRequestHandler {
	return &IdentityRequestHandler{next: next, trusted: trusted}
}

func (h *IdentityRequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, present := r.Header[http.CanonicalHeaderKey("Idempotency-Key")]; present &&
		(r.URL.Path == "/v1/accounts" || r.URL.Path == "/v1/unverified-account-claims" || r.URL.Path == "/v1/password-sessions" || r.URL.Path == "/v1/session-refreshes") {
		http.Error(w, "idempotency-key is not supported", http.StatusBadRequest)
		return
	}
	source, err := SourceAddress(r.RemoteAddr, r.Header, h.trusted)
	if err != nil {
		http.Error(w, "invalid source address", http.StatusBadRequest)
		return
	}
	h.next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sourceContextKey{}, source)))
}
