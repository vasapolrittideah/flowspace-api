package google_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/google"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

const callbackURL = "https://api.example.com/v1/provider-login-callbacks/google"

type googleFixture struct {
	server      *httptest.Server
	key, other  *rsa.PrivateKey
	tokenStatus int
	idToken     string
	form        url.Values
}

func newGoogleFixture(t *testing.T) *googleFixture {
	t.Helper()
	f := &googleFixture{tokenStatus: http.StatusOK}
	var err error
	if f.key, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatal(err)
	}
	if f.other, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
		t.Fatal(err)
	}
	keys := &oidctest.Server{PublicKeys: []oidctest.PublicKey{{PublicKey: f.key.Public(), KeyID: "key-1", Algorithm: oidc.RS256}}}
	mux := http.NewServeMux()
	mux.Handle("GET /keys", keys)
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		f.form = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.tokenStatus)
		if f.tokenStatus != http.StatusOK {
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "provider-access", "token_type": "Bearer", "id_token": f.idToken})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *googleFixture) identity() *google.ProviderIdentity {
	return google.NewProviderIdentity(context.Background(), "client-id", "client-secret", google.Endpoints{
		CodeExchange: f.server.URL + "/token", Keys: f.server.URL + "/keys", Issuer: f.server.URL,
	}, f.server.Client())
}

func (f *googleFixture) claims() map[string]any {
	return map[string]any{
		"iss": f.server.URL, "aud": "client-id", "sub": "google-subject", "nonce": "nonce",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"email": "user@example.com", "email_verified": true, "hd": "example.com",
	}
}

func (f *googleFixture) sign(t *testing.T, key *rsa.PrivateKey, claims map[string]any) {
	t.Helper()
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	f.idToken = oidctest.SignIDToken(key, "key-1", oidc.RS256, string(raw))
}

func exchange() outbound.ProviderCodeExchange {
	return outbound.ProviderCodeExchange{Code: "provider-code", CodeVerifier: "pkce-verifier", Nonce: "nonce", CallbackURL: callbackURL}
}

func TestVerifyProviderIdentityExchangesCodeAndChecksIDToken(t *testing.T) {
	f := newGoogleFixture(t)
	f.sign(t, f.key, f.claims())
	identity, err := f.identity().VerifyProviderIdentity(t.Context(), exchange())
	if err != nil {
		t.Fatal(err)
	}
	want := outbound.ProviderIdentity{Subject: "google-subject", Email: "user@example.com", EmailVerified: true, HostedDomain: "example.com"}
	if identity != want {
		t.Fatalf("identity = %+v", identity)
	}
	for field, value := range map[string]string{
		"grant_type": "authorization_code", "code": "provider-code", "code_verifier": "pkce-verifier",
		"redirect_uri": callbackURL, "client_id": "client-id", "client_secret": "client-secret",
	} {
		if f.form.Get(field) != value {
			t.Fatalf("token request %s = %q", field, f.form.Get(field))
		}
	}
}

func TestVerifyProviderIdentityTreatsMissingEmailVerificationAsUnverified(t *testing.T) {
	f := newGoogleFixture(t)
	claims := f.claims()
	delete(claims, "email_verified")
	f.sign(t, f.key, claims)
	identity, err := f.identity().VerifyProviderIdentity(t.Context(), exchange())
	if err != nil || identity.EmailVerified {
		t.Fatalf("identity = %+v, %v", identity, err)
	}
}

func TestVerifyProviderIdentityRejectsInvalidProof(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(map[string]any)
		other  bool
	}{
		{"wrong signature", nil, true},
		{"wrong issuer", func(c map[string]any) { c["iss"] = "https://attacker.example" }, false},
		{"wrong audience", func(c map[string]any) { c["aud"] = "other-client" }, false},
		{"expired", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }, false},
		{"wrong nonce", func(c map[string]any) { c["nonce"] = "other-nonce" }, false},
		{"missing nonce", func(c map[string]any) { delete(c, "nonce") }, false},
		{"empty subject", func(c map[string]any) { c["sub"] = "" }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newGoogleFixture(t)
			claims := f.claims()
			if test.change != nil {
				test.change(claims)
			}
			key := f.key
			if test.other {
				key = f.other
			}
			f.sign(t, key, claims)
			if identity, err := f.identity().VerifyProviderIdentity(t.Context(), exchange()); !errors.Is(err, domain.ErrInvalidProviderProof) {
				t.Fatalf("identity %+v, error %v", identity, err)
			}
		})
	}
}

func TestVerifyProviderIdentityClassifiesTokenEndpointFailures(t *testing.T) {
	for _, test := range []struct {
		status int
		want   error
	}{
		{http.StatusBadRequest, domain.ErrInvalidProviderProof},
		{http.StatusInternalServerError, domain.ErrProviderUnavailable},
	} {
		f := newGoogleFixture(t)
		f.tokenStatus = test.status
		if _, err := f.identity().VerifyProviderIdentity(t.Context(), exchange()); !errors.Is(err, test.want) {
			t.Fatalf("status %d error = %v, want %v", test.status, err, test.want)
		}
	}

	f := newGoogleFixture(t)
	f.sign(t, f.key, f.claims())
	identity := f.identity()
	f.server.Close()
	if _, err := identity.VerifyProviderIdentity(t.Context(), exchange()); !errors.Is(err, domain.ErrProviderUnavailable) {
		t.Fatalf("closed provider error = %v", err)
	}
}

func TestVerifyProviderIdentityRejectsMissingIDToken(t *testing.T) {
	f := newGoogleFixture(t)
	if _, err := f.identity().VerifyProviderIdentity(t.Context(), exchange()); !errors.Is(err, domain.ErrInvalidProviderProof) {
		t.Fatalf("missing ID token error = %v", err)
	}
}
