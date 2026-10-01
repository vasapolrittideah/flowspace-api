package github_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/github"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

const callbackURL = "https://api.example.com/v1/provider-login-callbacks/github"

type githubFixture struct {
	server                               *httptest.Server
	tokenStatus, userStatus, emailStatus int
	tokenBody, userBody, emailBody       string
	form                                 url.Values
	authorization                        []string
}

func newGitHubFixture(t *testing.T) *githubFixture {
	t.Helper()
	f := &githubFixture{
		tokenStatus: http.StatusOK, userStatus: http.StatusOK, emailStatus: http.StatusOK,
		tokenBody: `{"access_token":"provider-access","token_type":"bearer","scope":"user:email"}`,
		userBody:  `{"id":12345,"login":"octocat","email":"profile@example.com"}`,
		emailBody: `[{"email":"other@example.com","primary":false,"verified":true},{"email":"Primary@Example.com","primary":true,"verified":true}]`,
	}
	respond := func(w http.ResponseWriter, status int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		f.form = r.PostForm
		respond(w, f.tokenStatus, f.tokenBody)
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		f.authorization = append(f.authorization, r.Header.Get("Authorization"))
		respond(w, f.userStatus, f.userBody)
	})
	mux.HandleFunc("GET /user/emails", func(w http.ResponseWriter, r *http.Request) {
		f.authorization = append(f.authorization, r.Header.Get("Authorization"))
		respond(w, f.emailStatus, f.emailBody)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *githubFixture) identity() *github.ProviderIdentity {
	return github.NewProviderIdentity("client-id", "client-secret", github.Endpoints{
		CodeExchange: f.server.URL + "/login/oauth/access_token", User: f.server.URL + "/user", Emails: f.server.URL + "/user/emails",
	}, f.server.Client())
}

func exchange() outbound.ProviderCodeExchange {
	return outbound.ProviderCodeExchange{Code: "provider-code", CodeVerifier: "pkce-verifier", CallbackURL: callbackURL}
}

func TestVerifyProviderIdentityUsesUserIDAndPrimaryVerifiedEmail(t *testing.T) {
	f := newGitHubFixture(t)
	identity, err := f.identity().VerifyProviderIdentity(t.Context(), exchange())
	if err != nil {
		t.Fatal(err)
	}
	if identity != (outbound.ProviderIdentity{Subject: "12345", Email: "Primary@Example.com", EmailVerified: true}) {
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
	if len(f.authorization) != 2 || f.authorization[0] != "Bearer provider-access" || f.authorization[1] != "Bearer provider-access" {
		t.Fatalf("authorization = %q", f.authorization)
	}
}

func TestVerifyProviderIdentityDoesNotTreatOtherEmailsAsProof(t *testing.T) {
	for name, body := range map[string]string{
		"unverified primary":   `[{"email":"primary@example.com","primary":true,"verified":false},{"email":"other@example.com","primary":false,"verified":true}]`,
		"no primary":           `[{"email":"other@example.com","primary":false,"verified":true}]`,
		"empty list":           `[]`,
		"malformed email list": `{"email":"primary@example.com"}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newGitHubFixture(t)
			f.emailBody = body
			identity, err := f.identity().VerifyProviderIdentity(t.Context(), exchange())
			// The profile email is never used as verified proof.
			if err != nil || identity != (outbound.ProviderIdentity{Subject: "12345"}) {
				t.Fatalf("identity = %+v, %v", identity, err)
			}
		})
	}
}

func TestVerifyProviderIdentityKeepsTheSubjectWhenTheEmailEndpointFails(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		f := newGitHubFixture(t)
		f.emailStatus = status
		identity, err := f.identity().VerifyProviderIdentity(t.Context(), exchange())
		if err != nil || identity != (outbound.ProviderIdentity{Subject: "12345"}) {
			t.Fatalf("status %d identity = %+v, %v", status, identity, err)
		}
	}
}

func TestVerifyProviderIdentityRejectsInvalidProof(t *testing.T) {
	for name, change := range map[string]func(*githubFixture){
		"rejected code":       func(f *githubFixture) { f.tokenBody = `{"error":"bad_verification_code"}` },
		"rejected client":     func(f *githubFixture) { f.tokenStatus = http.StatusUnauthorized },
		"rejected user token": func(f *githubFixture) { f.userStatus = http.StatusUnauthorized },
		"missing user ID":     func(f *githubFixture) { f.userBody = `{"login":"octocat"}` },
		"zero user ID":        func(f *githubFixture) { f.userBody = `{"id":0}` },
		"string user ID":      func(f *githubFixture) { f.userBody = `{"id":"12345"}` },
		"malformed user":      func(f *githubFixture) { f.userBody = `not json` },
	} {
		t.Run(name, func(t *testing.T) {
			f := newGitHubFixture(t)
			change(f)
			identity, err := f.identity().VerifyProviderIdentity(t.Context(), exchange())
			if !errors.Is(err, domain.ErrInvalidProviderProof) || identity != (outbound.ProviderIdentity{}) {
				t.Fatalf("identity = %+v, %v", identity, err)
			}
		})
	}
}

func TestVerifyProviderIdentityReportsProviderOutages(t *testing.T) {
	for name, change := range map[string]func(*githubFixture){
		"token endpoint": func(f *githubFixture) { f.tokenStatus = http.StatusBadGateway },
		"user endpoint":  func(f *githubFixture) { f.userStatus = http.StatusServiceUnavailable },
	} {
		t.Run(name, func(t *testing.T) {
			f := newGitHubFixture(t)
			change(f)
			_, err := f.identity().VerifyProviderIdentity(t.Context(), exchange())
			if !errors.Is(err, domain.ErrProviderUnavailable) || errors.Is(err, domain.ErrInvalidProviderProof) {
				t.Fatalf("err = %v", err)
			}
			if strings.Contains(err.Error(), "provider-access") || strings.Contains(err.Error(), "client-secret") {
				t.Fatalf("error exposes a secret: %v", err)
			}
		})
	}
}

func TestVerifyProviderIdentityErrorsDoNotExposeProviderResponses(t *testing.T) {
	f := newGitHubFixture(t)
	f.userStatus = http.StatusInternalServerError
	f.userBody = `{"message":"provider-access"}`
	_, err := f.identity().VerifyProviderIdentity(t.Context(), exchange())
	if err == nil || strings.Contains(err.Error(), "provider-access") {
		t.Fatalf("err = %v", err)
	}
}
