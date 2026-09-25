package bootstrap

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	httptransport "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/crypto"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/hibp"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/postgres"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/out/token"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
)

const requestTimeout = 5 * time.Second

type APIServer struct {
	public   *http.Server
	internal *http.Server
	pool     *pgxpool.Pool
	logger   *zap.Logger
}

func NewAPIServer(ctx context.Context, config APIConfig, logger *zap.Logger) (*APIServer, error) {
	privateKey, err := readSigningKey(config.SigningKeyFile)
	if err != nil {
		return nil, err
	}
	verifierKey, err := readKey(config.CodeVerifierKeyFile)
	if err != nil {
		return nil, err
	}
	deliveryKey, err := readKey(config.DeliveryKeyFile)
	if err != nil {
		return nil, err
	}
	trusted, err := loadTrustedProxies(config.TrustedProxyCIDRs)
	if err != nil {
		return nil, err
	}
	signer, err := token.NewSigner(privateKey, config.SigningKeyID, config.TokenIssuer, config.TokenAudience)
	if err != nil {
		return nil, err
	}
	verifier, err := token.NewVerifier(ed25519.PublicKey(privateKey[ed25519.SeedSize:]), config.SigningKeyID, config.TokenIssuer, config.TokenAudience)
	if err != nil {
		return nil, err
	}
	protector, err := crypto.NewDeliveryProtector(deliveryKey, 1)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(ctx, string(config.DatabaseURL))
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("identity database unavailable")
	}
	if _, err := pool.Exec(ctx, "SELECT 1 FROM identity_outbox_events LIMIT 0"); err != nil {
		pool.Close()
		return nil, errors.New("identity schema unavailable")
	}
	accountRepo := postgres.NewAccountRepository(pool)
	limits := app.NewLimitService(postgres.NewLimitRepository(pool))
	checkPassword := hibp.NewPasswordChecker(&http.Client{Timeout: 4 * time.Second}).Compromised
	handler := httptransport.NewIdentityHandler(
		app.NewSignupService(accountRepo, signer, protector, limits.Signup, checkPassword, verifierKey),
		app.NewVerificationCodeService(accountRepo, protector, limits.CodeRequest, limits.WrongCode, limits.AccountWrongCode, verifierKey),
		app.NewClaimCodeService(accountRepo, protector, limits.CodeRequest, verifierKey),
		app.NewAccountClaimService(accountRepo, signer, limits.WrongCode, limits.AccountWrongCode, checkPassword, verifierKey),
		verifier, trusted,
	)
	public, err := newPublicHandler(ctx, handler, logger, trusted)
	if err != nil {
		pool.Close()
		return nil, err
	}
	jwks, err := json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: privateKey.Public(), KeyID: config.SigningKeyID, Algorithm: string(jose.EdDSA), Use: "sig",
	}}})
	if err != nil {
		pool.Close()
		return nil, errors.New("invalid public signing key")
	}
	internal := http.NewServeMux()
	internal.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	internal.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		var pending int64
		err := pool.QueryRow(checkCtx, "SELECT count(*) FROM identity_outbox_events WHERE published_at IS NULL").Scan(&pending)
		if err != nil || pending >= int64(config.OutboxReadyMaxPending) {
			http.Error(w, "identity unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	internal.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	})
	return &APIServer{
		public: newHTTPServer(config.HTTPAddress, public), internal: newHTTPServer(config.InternalHTTPAddress, internal),
		pool: pool, logger: logger,
	}, nil
}

func loadTrustedProxies(value string) ([]netip.Prefix, error) {
	if value == "" {
		return nil, nil
	}
	trusted, err := httptransport.TrustedProxies(strings.Split(value, ","))
	if err != nil {
		return nil, errors.New("invalid trusted proxy CIDR")
	}
	return trusted, nil
}

func newHTTPServer(address string, handler http.Handler) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{
		Addr: address, Handler: handler, Protocols: protocols,
		ReadHeaderTimeout: requestTimeout, ReadTimeout: requestTimeout, WriteTimeout: requestTimeout,
		IdleTimeout: 30 * time.Second,
	}
}

func (s *APIServer) Run(ctx context.Context) error {
	defer s.pool.Close()
	publicListener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.public.Addr)
	if err != nil {
		return err
	}
	defer func() { _ = publicListener.Close() }()
	internalListener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.internal.Addr)
	if err != nil {
		return err
	}
	defer func() { _ = internalListener.Close() }()
	s.logger.Info("process_listening", zap.String("address", s.public.Addr))
	results := make(chan error, 2)
	go func() { results <- s.public.Serve(publicListener) }()
	go func() { results <- s.internal.Serve(internalListener) }()
	var result error
	select {
	case result = <-results:
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()
	_ = s.public.Shutdown(shutdownCtx)
	_ = s.internal.Shutdown(shutdownCtx)
	if result == nil {
		result = <-results
	}
	if err := <-results; err != nil && !errors.Is(err, http.ErrServerClosed) && result == nil {
		result = err
	}
	if errors.Is(result, http.ErrServerClosed) {
		return nil
	}
	return result
}

func newPublicHandler(ctx context.Context, handler identityv1.IdentityServiceServer, logger *zap.Logger, trusted []netip.Prefix) (http.Handler, error) {
	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(1<<20), grpc.UnaryInterceptor(func(ctx context.Context, request any,
		info *grpc.UnaryServerInfo, next grpc.UnaryHandler,
	) (any, error) {
		started := time.Now()
		response, err := next(ctx, request)
		id := metadata.ValueFromIncomingContext(ctx, "x-request-id")
		requestID := ""
		if len(id) == 1 && validRequestID(id[0]) {
			requestID = id[0]
		}
		logger.Info("identity_rpc", zap.String("request_id", requestID), zap.String("operation", info.FullMethod),
			zap.String("outcome", status.Code(err).String()), zap.Duration("duration", time.Since(started)))
		return response, err
	}))
	identityv1.RegisterIdentityServiceServer(grpcServer, handler)
	gateway := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
		switch {
		case strings.EqualFold(key, "Idempotency-Key"):
			return "idempotency-key", true
		case strings.EqualFold(key, "X-Request-ID"):
			return "x-request-id", true
		}
		return runtime.DefaultHeaderMatcher(key)
	}))
	if err := identityv1.RegisterIdentityServiceHandlerServer(ctx, gateway, handler); err != nil {
		return nil, err
	}
	rest := httptransport.NewIdentityRequestHandler(gateway, trusted)
	public := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		rest.ServeHTTP(w, r)
	})
	return observeRequests(public, logger), nil
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func observeRequests(next http.Handler, logger *zap.Logger) http.Handler {
	propagator := propagation.TraceContext{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := r.Header.Get("X-Request-ID")
		if !validRequestID(requestID) {
			requestID = rand.Text()
		}
		w.Header().Set("X-Request-ID", requestID)
		r.Header.Set("X-Request-ID", requestID)
		observed := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := otel.Tracer("flowspace/identity/api").Start(ctx, "identity.http")
		defer span.End()
		next.ServeHTTP(observed, r.WithContext(ctx))
		logger.Info("identity_request", zap.String("request_id", requestID), zap.String("trace_id", traceID(ctx)), zap.String("operation", safeOperation(r)),
			zap.Int("status", observed.status), zap.Duration("duration", time.Since(started)))
	})
}

func traceID(ctx context.Context) string {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		return spanContext.TraceID().String()
	}
	return ""
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func safeOperation(r *http.Request) string {
	for _, path := range []string{
		"/v1/accounts", "/v1/email-verification-codes", "/v1/email-verifications",
		"/v1/unverified-account-claim-codes", "/v1/unverified-account-claims",
	} {
		if r.URL.Path == path {
			return r.Method + " " + path
		}
	}
	if r.ProtoMajor == 2 && strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/grpc") {
		return "identity-rpc"
	}
	return "unknown"
}
