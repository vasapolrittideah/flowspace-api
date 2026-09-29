package authn

import (
	"context"
	"crypto/ed25519"
	"errors"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrUnavailable     = errors.New("identity unavailable")
)

type Claims struct {
	Subject   string
	SessionID string
}

func VerifyAccessToken(raw, issuer, audience string, keyFor func(string) (ed25519.PublicKey, error)) (Claims, error) {
	token, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.EdDSA})
	if err != nil || len(token.Headers) != 1 || token.Headers[0].KeyID == "" || token.Headers[0].ExtraHeaders[jose.HeaderKey("typ")] != "at+jwt" {
		return Claims{}, ErrUnauthenticated
	}
	key, err := keyFor(token.Headers[0].KeyID)
	if err != nil {
		return Claims{}, err
	}
	if len(key) != ed25519.PublicKeySize {
		return Claims{}, ErrUnauthenticated
	}
	var standard jwt.Claims
	var extra struct {
		SessionID string `json:"sid"`
	}
	if err := token.Claims(key, &standard, &extra); err != nil || standard.Subject == "" || extra.SessionID == "" ||
		standard.ID == "" || standard.IssuedAt == nil || standard.Expiry == nil ||
		standard.ValidateWithLeeway(jwt.Expected{Issuer: issuer, AnyAudience: jwt.Audience{audience}}, 0) != nil {
		return Claims{}, ErrUnauthenticated
	}
	return Claims{Subject: standard.Subject, SessionID: extra.SessionID}, nil
}

func CheckSession(ctx context.Context, claims Claims, check func(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error)) (bool, error) {
	if claims.Subject == "" || claims.SessionID == "" {
		return false, ErrUnauthenticated
	}
	response, err := check(ctx, &identityv1.CheckSessionRequest{Subject: claims.Subject, SessionId: claims.SessionID})
	if status.Code(err) == codes.Unauthenticated {
		return false, ErrUnauthenticated
	}
	if err != nil || response == nil {
		return false, ErrUnavailable
	}
	return response.GetEmailVerified(), nil
}
