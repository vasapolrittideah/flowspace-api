package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"golang.org/x/oauth2"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

// Endpoints are the GitHub code exchange, authenticated user, and email locations.
type Endpoints struct {
	CodeExchange string
	User         string
	Emails       string
}

// DefaultEndpoints are GitHub's published OAuth and REST endpoints.
var DefaultEndpoints = Endpoints{
	CodeExchange: "https://github.com/login/oauth/access_token",
	User:         "https://api.github.com/user",
	Emails:       "https://api.github.com/user/emails",
}

// ProviderIdentity exchanges a GitHub authorization code and reads the
// authenticated user. The provider access token never leaves this adapter.
type ProviderIdentity struct {
	clientID, clientSecret string
	endpoints              Endpoints
	client                 *http.Client
}

var _ outbound.ProviderIdentityVerifier = (*ProviderIdentity)(nil)

func NewProviderIdentity(clientID, clientSecret string, endpoints Endpoints, client *http.Client) *ProviderIdentity {
	return &ProviderIdentity{clientID: clientID, clientSecret: clientSecret, endpoints: endpoints, client: client}
}

var errRejected = errors.New("github rejected the request")

func (p *ProviderIdentity) VerifyProviderIdentity(ctx context.Context, exchange outbound.ProviderCodeExchange) (outbound.ProviderIdentity, error) {
	config := oauth2.Config{
		ClientID: p.clientID, ClientSecret: p.clientSecret, RedirectURL: exchange.CallbackURL,
		Endpoint: oauth2.Endpoint{TokenURL: p.endpoints.CodeExchange, AuthStyle: oauth2.AuthStyleInParams},
	}
	token, err := config.Exchange(context.WithValue(ctx, oauth2.HTTPClient, p.client), exchange.Code, oauth2.VerifierOption(exchange.CodeVerifier))
	if err != nil {
		// GitHub reports a rejected code with an error body and status 200.
		var rejected *oauth2.RetrieveError
		if errors.As(err, &rejected) && rejected.Response != nil && rejected.Response.StatusCode < http.StatusInternalServerError {
			return outbound.ProviderIdentity{}, domain.ErrInvalidProviderProof
		}
		return outbound.ProviderIdentity{}, errors.Join(domain.ErrProviderUnavailable, ctx.Err())
	}
	var user struct {
		ID int64 `json:"id"`
	}
	if err := p.get(ctx, p.endpoints.User, token.AccessToken, &user); err != nil {
		if errors.Is(err, errRejected) {
			return outbound.ProviderIdentity{}, domain.ErrInvalidProviderProof
		}
		return outbound.ProviderIdentity{}, errors.Join(domain.ErrProviderUnavailable, err)
	}
	if user.ID <= 0 {
		return outbound.ProviderIdentity{}, domain.ErrInvalidProviderProof
	}
	identity := outbound.ProviderIdentity{Subject: strconv.FormatInt(user.ID, 10)}
	// The user endpoint already proved the subject, so an email failure only
	// leaves the identity without email proof. A known link can still log in.
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if p.get(ctx, p.endpoints.Emails, token.AccessToken, &emails) == nil {
		for _, email := range emails {
			if email.Primary && email.Verified {
				identity.Email, identity.EmailVerified = email.Email, true
			}
		}
	}
	return identity, nil
}

// get reads one authenticated JSON resource. Its errors never include the
// response body or the access token.
func (p *ProviderIdentity) get(ctx context.Context, endpoint, accessToken string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := p.client.Do(request)
	if err != nil {
		return errors.Join(errors.New("github request failed"), ctx.Err())
	}
	defer func() { _ = response.Body.Close() }()
	switch {
	case response.StatusCode >= http.StatusInternalServerError:
		return fmt.Errorf("github status %d", response.StatusCode)
	case response.StatusCode != http.StatusOK:
		return errRejected
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target); err != nil {
		return errRejected
	}
	return nil
}
