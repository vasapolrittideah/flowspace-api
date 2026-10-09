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
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/internal/authn"
	"github.com/vasapolrittideah/flowspace-api/internal/requestid"
	"github.com/vasapolrittideah/flowspace-api/internal/tracing"
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
	conn, err := dialSession(config.SessionAddress, credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: roots, ServerName: config.SessionServerName,
	}))
	if err != nil {
		return nil, errors.New("invalid Identity session address")
	}
	return &TokenVerifier{
		jwksURL: config.JWKSURL, issuer: config.Issuer, audience: config.Audience,
		client: &http.Client{Timeout: 2 * time.Second},
		check:  newSessionCheck(conn),
		conn:   conn,
	}, nil
}

// dialSession returns the connection for session checks. Each call on it
// gets a client span and a client duration, and sends the correlation
// metadata. An instrument error goes to the global error handler, because
// telemetry never stops a process.
func dialSession(address string, transport credentials.TransportCredentials, options ...grpc.DialOption) (*grpc.ClientConn, error) {
	durations, err := otel.Meter("flowspace/workspace/api").Float64Histogram("rpc.client.call.duration", metric.WithUnit("s"),
		metric.WithDescription("Duration of each CheckSession call from Workspace"))
	if err != nil {
		otel.Handle(err)
	}
	return grpc.NewClient(address, append([]grpc.DialOption{
		grpc.WithTransportCredentials(transport), grpc.WithUnaryInterceptor(observeSessionCheck(durations)),
	}, options...)...)
}

func newSessionCheck(conn *grpc.ClientConn) func(context.Context, *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
	client := identityv1.NewIdentityServiceClient(conn)
	return func(ctx context.Context, request *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
		return client.CheckSession(ctx, request)
	}
}

// observeSessionCheck creates the client span of a session check under the
// Workspace request span, and records its duration in durations. It sends
// the trace context of the client span and the request ID of the Workspace
// request as gRPC metadata.
func observeSessionCheck(durations metric.Float64Histogram) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, request, reply any, conn *grpc.ClientConn,
		invoker grpc.UnaryInvoker, options ...grpc.CallOption,
	) error {
		started := time.Now()
		err := traceSessionCheck(ctx, method, request, reply, conn, invoker, options...)
		durations.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(
			semconv.RPCMethod(strings.TrimPrefix(method, "/")), tracing.GRPCStatusCode(status.Code(err))))
		return err
	}
}

// traceSessionCheck creates the client span of a session check under the
// Workspace request span. It sends the trace context of the client span
// and the request ID of the Workspace request as gRPC metadata.
func traceSessionCheck(ctx context.Context, method string, request, reply any, conn *grpc.ClientConn,
	invoker grpc.UnaryInvoker, options ...grpc.CallOption,
) error {
	ctx, span := otel.Tracer("flowspace/workspace/api").Start(ctx, method, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	// Replace earlier correlation values, because Identity ignores a key
	// that has more than one value.
	existing, _ := metadata.FromOutgoingContext(ctx)
	outgoing := existing.Copy()
	outgoing.Delete("tracestate")
	outgoing.Delete("x-request-id")
	propagation.TraceContext{}.Inject(ctx, tracing.MetadataCarrier(outgoing))
	if id := requestid.FromIncoming(ctx); id != "" {
		outgoing.Set("x-request-id", id)
	}
	err := invoker(metadata.NewOutgoingContext(ctx, outgoing), method, request, reply, conn, options...)
	tracing.SetGRPCStatus(span, status.Code(err))
	return err
}

func (v *TokenVerifier) Close() error {
	if v.conn != nil {
		return v.conn.Close()
	}
	return nil
}

func (v *TokenVerifier) VerifyToken(ctx context.Context, raw string) (string, error) {
	if len(raw) == 0 || len(raw) > 8192 {
		return "", outbound.ErrUnauthenticated
	}
	claims, err := authn.VerifyAccessToken(raw, v.issuer, v.audience, func(keyID string) (ed25519.PublicKey, error) {
		return v.publicKey(ctx, keyID)
	})
	if err != nil {
		if errors.Is(err, authn.ErrUnavailable) {
			return "", outbound.ErrIdentityUnavailable
		}
		return "", outbound.ErrUnauthenticated
	}
	verified, err := authn.CheckSession(ctx, claims, v.check)
	if errors.Is(err, authn.ErrUnauthenticated) {
		return "", outbound.ErrUnauthenticated
	}
	if err != nil {
		return "", outbound.ErrIdentityUnavailable
	}
	if !verified {
		return "", outbound.ErrEmailUnverified
	}
	return claims.Subject, nil
}

func (v *TokenVerifier) publicKey(ctx context.Context, keyID string) (ed25519.PublicKey, error) {
	if keyID == "" {
		return nil, authn.ErrUnauthenticated
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, authn.ErrUnavailable
	}
	response, err := v.client.Do(request)
	if err != nil {
		return nil, authn.ErrUnavailable
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, authn.ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(body) > 65536 {
		return nil, authn.ErrUnavailable
	}
	var keys jose.JSONWebKeySet
	if err := json.Unmarshal(body, &keys); err != nil {
		return nil, authn.ErrUnavailable
	}
	for _, candidate := range keys.Keys {
		if candidate.KeyID == keyID && candidate.Algorithm == string(jose.EdDSA) && candidate.Use == "sig" {
			key, ok := candidate.Key.(ed25519.PublicKey)
			if ok && len(key) == ed25519.PublicKeySize {
				return key, nil
			}
		}
	}
	return nil, authn.ErrUnauthenticated
}
