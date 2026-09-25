package http

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
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

type IdentityHandler struct {
	identityv1.UnimplementedIdentityServiceServer
	signup       inbound.SignupService
	verification inbound.VerificationCodeService
	claimCodes   inbound.ClaimCodeService
	claims       inbound.AccountClaimService
	verifier     outbound.AccessTokenVerifier
	trusted      []netip.Prefix
}

var _ identityv1.IdentityServiceServer = (*IdentityHandler)(nil)

func NewIdentityHandler(signup inbound.SignupService, verification inbound.VerificationCodeService, claimCodes inbound.ClaimCodeService,
	claims inbound.AccountClaimService,
	verifier outbound.AccessTokenVerifier, trusted []netip.Prefix,
) *IdentityHandler {
	return &IdentityHandler{signup: signup, verification: verification, claimCodes: claimCodes, claims: claims, verifier: verifier, trusted: trusted}
}

func (h *IdentityHandler) CreateAccount(ctx context.Context, request *identityv1.CreateAccountRequest) (*identityv1.CreateAccountResponse, error) {
	if len(metadata.ValueFromIncomingContext(ctx, "idempotency-key")) != 0 {
		return nil, invalidSignupArgument("idempotency_key", "is not supported")
	}
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	source, err := h.sourceAddress(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := h.signup.CreateAccount(ctx, inbound.CreateAccountInput{
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

func (h *IdentityHandler) RequestEmailVerificationCode(ctx context.Context, request *identityv1.RequestEmailVerificationCodeRequest) (*identityv1.RequestEmailVerificationCodeResponse, error) {
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	identity, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	source, err := h.sourceAddress(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if h.verification == nil {
		return nil, status.Error(codes.Unavailable, "verification code unavailable")
	}
	if err := h.verification.RequestEmailVerificationCode(ctx, inbound.RequestEmailVerificationCodeInput{
		Subject: identity.Subject, SessionID: identity.SessionID, Source: source,
	}); err != nil {
		return nil, verificationCodeRPCError(err)
	}
	return &identityv1.RequestEmailVerificationCodeResponse{Accepted: true}, nil
}

func (h *IdentityHandler) VerifyEmail(ctx context.Context, request *identityv1.VerifyEmailRequest) (*identityv1.VerifyEmailResponse, error) {
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	identity, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	source, err := h.sourceAddress(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if h.verification == nil {
		return nil, status.Error(codes.Unavailable, "email verification unavailable")
	}
	if err := h.verification.VerifyEmail(ctx, inbound.VerifyEmailInput{
		Subject: identity.Subject, SessionID: identity.SessionID, Source: source, Code: request.GetCode(),
	}); err != nil {
		return nil, verifyEmailRPCError(err)
	}
	return &identityv1.VerifyEmailResponse{EmailVerified: true}, nil
}

func (h *IdentityHandler) RequestUnverifiedAccountClaimCode(ctx context.Context, request *identityv1.RequestUnverifiedAccountClaimCodeRequest) (*identityv1.RequestUnverifiedAccountClaimCodeResponse, error) {
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	source, err := h.sourceAddress(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if h.claimCodes == nil {
		return nil, status.Error(codes.Unavailable, "claim code unavailable")
	}
	if err := h.claimCodes.RequestUnverifiedAccountClaimCode(ctx, inbound.RequestClaimCodeInput{
		Email: request.GetEmail(), Source: source,
	}); err != nil {
		return nil, claimCodeRPCError(err)
	}
	return &identityv1.RequestUnverifiedAccountClaimCodeResponse{Accepted: true}, nil
}

func (h *IdentityHandler) ClaimUnverifiedAccount(ctx context.Context, request *identityv1.ClaimUnverifiedAccountRequest) (*identityv1.ClaimUnverifiedAccountResponse, error) {
	if len(metadata.ValueFromIncomingContext(ctx, "idempotency-key")) != 0 {
		return nil, invalidSignupArgument("idempotency_key", "is not supported")
	}
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	source, err := h.sourceAddress(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if h.claims == nil {
		return nil, status.Error(codes.Unavailable, "account claim unavailable")
	}
	result, err := h.claims.ClaimUnverifiedAccount(ctx, inbound.ClaimAccountInput{
		Email: request.GetEmail(), Code: request.GetCode(), NewPassword: request.GetNewPassword(), Source: source,
	})
	if err != nil {
		return nil, accountClaimRPCError(err)
	}
	return &identityv1.ClaimUnverifiedAccountResponse{
		Subject: result.Subject, EmailVerified: true, AccessToken: result.AccessToken,
		RefreshToken: result.RefreshToken, AccessTokenExpiresAt: timestamppb.New(result.AccessTokenExpiresAt),
	}, nil
}

func (h *IdentityHandler) authenticate(ctx context.Context) (outbound.AccessTokenIdentity, error) {
	authorization := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(authorization) != 1 {
		return outbound.AccessTokenIdentity{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	parts := strings.Fields(authorization[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || h.verifier == nil {
		return outbound.AccessTokenIdentity{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	identity, err := h.verifier.Verify(parts[1])
	if err != nil || identity.Subject == "" || identity.SessionID == "" {
		return outbound.AccessTokenIdentity{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	return identity, nil
}

func (h *IdentityHandler) sourceAddress(ctx context.Context) (string, error) {
	if source, ok := ctx.Value(sourceContextKey{}).(string); ok {
		return source, nil
	}
	connection, present := peer.FromContext(ctx)
	if !present || connection.Addr == nil {
		return "", status.Error(codes.InvalidArgument, "source address is required")
	}
	headers := http.Header{}
	for _, name := range []string{"Forwarded", "X-Real-IP", "X-Forwarded-For"} {
		for _, value := range metadata.ValueFromIncomingContext(ctx, name) {
			headers.Add(name, value)
		}
	}
	source, err := SourceAddress(connection.Addr.String(), headers, h.trusted)
	if err != nil {
		return "", status.Error(codes.InvalidArgument, "invalid source address")
	}
	return source, nil
}

func verificationCodeRPCError(err error) error {
	switch {
	case errors.Is(err, outbound.ErrUnauthenticated):
		return status.Error(codes.Unauthenticated, "authentication required")
	case errors.Is(err, app.ErrEmailAlreadyVerified):
		return status.Error(codes.FailedPrecondition, "email is already verified")
	case errors.Is(err, app.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "verification code limit exceeded")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	default:
		return status.Error(codes.Unavailable, "verification code unavailable")
	}
}

func verifyEmailRPCError(err error) error {
	switch {
	case errors.Is(err, outbound.ErrUnauthenticated):
		return status.Error(codes.Unauthenticated, "authentication required")
	case errors.Is(err, app.ErrInvalidVerificationCode):
		return status.Error(codes.InvalidArgument, "invalid verification code")
	case errors.Is(err, app.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "verification code limit exceeded")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	default:
		return status.Error(codes.Unavailable, "email verification unavailable")
	}
}

func claimCodeRPCError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidEmail):
		return invalidSignupArgument("email", "is invalid")
	case errors.Is(err, app.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "claim code limit exceeded")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	default:
		return status.Error(codes.Unavailable, "claim code unavailable")
	}
}

func accountClaimRPCError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidEmail):
		return invalidSignupArgument("email", "is invalid")
	case errors.Is(err, domain.ErrInvalidPassword):
		return invalidSignupArgument("new_password", "does not meet the policy")
	case errors.Is(err, app.ErrInvalidClaimCode):
		return status.Error(codes.InvalidArgument, "invalid claim code")
	case errors.Is(err, app.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "claim limit exceeded")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	default:
		return status.Error(codes.Unavailable, "account claim unavailable")
	}
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
		(r.URL.Path == "/v1/accounts" || r.URL.Path == "/v1/unverified-account-claims") {
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
