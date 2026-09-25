package http

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type sourceContextKey struct{}

type SignupHandler struct {
	identityv1.UnimplementedIdentityServiceServer
	service inbound.SignupService
	trusted []netip.Prefix
}

var _ identityv1.IdentityServiceServer = (*SignupHandler)(nil)

func NewSignupHandler(service inbound.SignupService, trusted []netip.Prefix) *SignupHandler {
	return &SignupHandler{service: service, trusted: trusted}
}

func (h *SignupHandler) CreateAccount(ctx context.Context, request *identityv1.CreateAccountRequest) (*identityv1.CreateAccountResponse, error) {
	if len(metadata.ValueFromIncomingContext(ctx, "idempotency-key")) != 0 {
		return nil, invalidSignupArgument("idempotency_key", "is not supported")
	}
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	source, ok := ctx.Value(sourceContextKey{}).(string)
	if !ok {
		connection, present := peer.FromContext(ctx)
		if !present || connection.Addr == nil {
			return nil, status.Error(codes.InvalidArgument, "source address is required")
		}
		headers := http.Header{}
		for _, name := range []string{"Forwarded", "X-Real-IP", "X-Forwarded-For"} {
			for _, value := range metadata.ValueFromIncomingContext(ctx, name) {
				headers.Add(name, value)
			}
		}
		var err error
		source, err = SourceAddress(connection.Addr.String(), headers, h.trusted)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid source address")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := h.service.CreateAccount(ctx, inbound.CreateAccountInput{
		Email: request.GetEmail(), Password: request.GetPassword(), Source: source,
	})
	if err != nil {
		return nil, signupRPCError(err)
	}
	unverified := false
	return &identityv1.CreateAccountResponse{
		Subject: result.Subject, EmailVerified: &unverified,
		AccessToken: result.AccessToken, RefreshToken: result.RefreshToken,
		AccessTokenExpiresAt: timestamppb.New(result.AccessTokenExpiresAt),
	}, nil
}

func signupRPCError(err error) error {
	switch {
	case errors.Is(err, outbound.ErrAccountExists):
		return status.Error(codes.AlreadyExists, "account already exists")
	case errors.Is(err, domain.ErrInvalidEmail):
		return invalidSignupArgument("email", "is invalid")
	case errors.Is(err, domain.ErrInvalidPassword):
		return invalidSignupArgument("password", "does not meet the policy")
	case errors.Is(err, app.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "signup limit exceeded")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	default:
		return status.Error(codes.Unavailable, "signup unavailable")
	}
}

func invalidSignupArgument(field, reason string) error {
	result, err := status.New(codes.InvalidArgument, "invalid request").WithDetails(&errdetails.BadRequest{
		FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: field, Description: reason}},
	})
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid request")
	}
	return result.Err()
}

// SignupRequestHandler supplies the connection source to the local REST gateway.
type SignupRequestHandler struct {
	next    http.Handler
	trusted []netip.Prefix
}

func NewSignupRequestHandler(next http.Handler, trusted []netip.Prefix) *SignupRequestHandler {
	return &SignupRequestHandler{next: next, trusted: trusted}
}

func (h *SignupRequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, present := r.Header[http.CanonicalHeaderKey("Idempotency-Key")]; present {
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
