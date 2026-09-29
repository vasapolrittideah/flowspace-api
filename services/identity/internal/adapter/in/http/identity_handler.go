package http

import (
	"context"
	"net/http"
	"net/netip"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type sourceContextKey struct{}

type IdentityHandler struct {
	identityv1.UnimplementedIdentityServiceServer
	signup           inbound.SignupService
	verification     inbound.RequestEmailVerificationCodeService
	verifyEmail      inbound.VerifyEmailService
	claimCodes       inbound.ClaimCodeService
	passwordRecovery inbound.PasswordRecoveryRequestService
	claims           inbound.AccountClaimService
	passwordLogin    inbound.PasswordLoginService
	refresh          inbound.RefreshSessionService
	currentLogout    inbound.LogoutCurrentSessionService
	allLogout        inbound.LogoutAllSessionsService
	verifier         outbound.AccessTokenVerifier
	trusted          []netip.Prefix
}

func (h *IdentityHandler) WithPasswordLogin(service inbound.PasswordLoginService) *IdentityHandler {
	h.passwordLogin = service
	return h
}

func (h *IdentityHandler) WithPasswordRecoveryRequest(service inbound.PasswordRecoveryRequestService) *IdentityHandler {
	h.passwordRecovery = service
	return h
}

func (h *IdentityHandler) WithVerifyEmail(service inbound.VerifyEmailService) *IdentityHandler {
	h.verifyEmail = service
	return h
}

func (h *IdentityHandler) WithRefreshSession(service inbound.RefreshSessionService) *IdentityHandler {
	h.refresh = service
	return h
}

func (h *IdentityHandler) WithCurrentSessionLogout(service inbound.LogoutCurrentSessionService) *IdentityHandler {
	h.currentLogout = service
	return h
}

func (h *IdentityHandler) WithAllSessionLogout(service inbound.LogoutAllSessionsService) *IdentityHandler {
	h.allLogout = service
	return h
}

var _ identityv1.IdentityServiceServer = (*IdentityHandler)(nil)

func NewIdentityHandler(signup inbound.SignupService, verification inbound.RequestEmailVerificationCodeService, claimCodes inbound.ClaimCodeService,
	claims inbound.AccountClaimService,
	verifier outbound.AccessTokenVerifier, trusted []netip.Prefix,
) *IdentityHandler {
	return &IdentityHandler{signup: signup, verification: verification, claimCodes: claimCodes, claims: claims, verifier: verifier, trusted: trusted}
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

func invalidSignupArgument(field, reason string) error {
	result, err := status.New(codes.InvalidArgument, "invalid request").WithDetails(&errdetails.BadRequest{
		FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: field, Description: reason}},
	})
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid request")
	}
	return result.Err()
}
