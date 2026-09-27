package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	outbound "github.com/vasapolrittideah/flowspace-api/services/workspace/internal/port/out"
)

type Config struct {
	JWKSURL           string
	Issuer            string
	Audience          string
	SessionAddress    string
	SessionServerName string
	ClientCertFile    string
	ClientKeyFile     string
	CAFile            string
}

type TokenVerifier struct {
	jwksURL  string
	issuer   string
	audience string
	client   *http.Client
	check    func(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error)
	conn     *grpc.ClientConn
}

var _ outbound.TokenVerifier = (*TokenVerifier)(nil)

func NewTokenVerifier(config Config) (*TokenVerifier, error) {
	parsed, err := url.Parse(config.JWKSURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		config.Issuer == "" || config.Audience == "" || config.SessionAddress == "" || config.SessionServerName == "" {
		return nil, errors.New("invalid Identity verifier configuration")
	}
	certificate, err := tls.LoadX509KeyPair(config.ClientCertFile, config.ClientKeyFile)
	if err != nil {
		return nil, errors.New("invalid Identity client certificate")
	}
	caPEM, err := os.ReadFile(config.CAFile)
	if err != nil {
		return nil, errors.New("invalid Identity CA certificate")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("invalid Identity CA certificate")
	}
	conn, err := grpc.NewClient(config.SessionAddress, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: roots, ServerName: config.SessionServerName,
	})))
	if err != nil {
		return nil, errors.New("invalid Identity session address")
	}
	client := identityv1.NewIdentityServiceClient(conn)
	return &TokenVerifier{
		jwksURL: config.JWKSURL, issuer: config.Issuer, audience: config.Audience,
		client: &http.Client{Timeout: 2 * time.Second},
		check: func(ctx context.Context, request *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
			return client.CheckSession(ctx, request)
		},
		conn: conn,
	}, nil
}

func (v *TokenVerifier) Close() error {
	if v.conn != nil {
		return v.conn.Close()
	}
	return nil
}

func (v *TokenVerifier) VerifyToken(ctx context.Context, raw string) (string, error) {
	subject, sessionID, err := v.verifyClaims(ctx, raw)
	if err != nil {
		return "", err
	}
	response, err := v.check(ctx, &identityv1.CheckSessionRequest{Subject: subject, SessionId: sessionID})
	if status.Code(err) == codes.Unauthenticated {
		return "", outbound.ErrUnauthenticated
	}
	if err != nil || response == nil {
		return "", outbound.ErrIdentityUnavailable
	}
	if !response.GetEmailVerified() {
		return "", outbound.ErrEmailUnverified
	}
	return subject, nil
}

func (v *TokenVerifier) verifyClaims(ctx context.Context, raw string) (string, string, error) {
	if len(raw) == 0 || len(raw) > 8192 {
		return "", "", outbound.ErrUnauthenticated
	}
	token, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.EdDSA})
	if err != nil || len(token.Headers) != 1 || token.Headers[0].ExtraHeaders[jose.HeaderKey("typ")] != "at+jwt" {
		return "", "", outbound.ErrUnauthenticated
	}
	key, err := v.publicKey(ctx, token.Headers[0].KeyID)
	if err != nil {
		return "", "", err
	}
	var claims jwt.Claims
	var extra struct {
		SessionID string `json:"sid"`
	}
	if err := token.Claims(key, &claims, &extra); err != nil || claims.Subject == "" || extra.SessionID == "" ||
		claims.ID == "" || claims.IssuedAt == nil || claims.Expiry == nil ||
		claims.ValidateWithLeeway(jwt.Expected{Issuer: v.issuer, AnyAudience: jwt.Audience{v.audience}}, 0) != nil {
		return "", "", outbound.ErrUnauthenticated
	}
	return claims.Subject, extra.SessionID, nil
}

func (v *TokenVerifier) publicKey(ctx context.Context, keyID string) (ed25519.PublicKey, error) {
	if keyID == "" {
		return nil, outbound.ErrUnauthenticated
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, outbound.ErrIdentityUnavailable
	}
	response, err := v.client.Do(request)
	if err != nil {
		return nil, outbound.ErrIdentityUnavailable
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, outbound.ErrIdentityUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(body) > 65536 {
		return nil, outbound.ErrIdentityUnavailable
	}
	var keys jose.JSONWebKeySet
	if err := json.Unmarshal(body, &keys); err != nil {
		return nil, outbound.ErrIdentityUnavailable
	}
	for _, candidate := range keys.Keys {
		if candidate.KeyID == keyID && candidate.Algorithm == string(jose.EdDSA) && candidate.Use == "sig" {
			key, ok := candidate.Key.(ed25519.PublicKey)
			if ok && len(key) == ed25519.PublicKeySize {
				return key, nil
			}
		}
	}
	return nil, outbound.ErrUnauthenticated
}
