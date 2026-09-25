package http

import "net/http"

type SessionHandler struct{ next http.Handler }

// NewSessionHandler wraps a token-issuance route with a no-store response header.
func NewSessionHandler(next http.Handler) *SessionHandler {
	return &SessionHandler{next: next}
}

func (h *SessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	h.next.ServeHTTP(w, r)
}
