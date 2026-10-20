package http

import (
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func ReauthenticationRPCError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, app.ErrInvalidCredentials):
		return status.Error(codes.InvalidArgument, "invalid credentials")
	case errors.Is(err, domain.ErrInvalidPassword):
		return invalidSignupArgument("password", "is invalid")
	case errors.Is(err, app.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "re-authentication limit exceeded")
	case errors.Is(err, app.ErrReauthenticationRequired):
		result, detailErr := status.New(codes.PermissionDenied, "re-authentication required").WithDetails(&errdetails.ErrorInfo{
			Reason: "REAUTHENTICATION_REQUIRED", Domain: "identity.flowspace",
		})
		if detailErr != nil {
			return status.Error(codes.PermissionDenied, "re-authentication required")
		}
		return result.Err()
	case errors.Is(err, outbound.ErrUnauthenticated):
		return status.Error(codes.Unauthenticated, "authentication required")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	default:
		return status.Error(codes.Unavailable, "re-authentication unavailable")
	}
}

func (h *IdentityHandler) ReauthenticateWithPassword(ctx context.Context, request *identityv1.ReauthenticateWithPasswordRequest) (*identityv1.ReauthenticateWithPasswordResponse, error) {
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	identity, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := domain.NormalizeLoginPassword(request.GetPassword()); err != nil {
		return nil, invalidSignupArgument("password", "is invalid")
	}
	source, err := h.sourceAddress(ctx)
	if err != nil {
		return nil, err
	}
	if h.passwordReauthentication == nil {
		return nil, ReauthenticationRPCError(app.ErrReauthenticationUnavailable)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := h.passwordReauthentication.ReauthenticateWithPassword(ctx, inbound.ReauthenticateWithPasswordInput{Subject: identity.Subject, SessionID: identity.SessionID, Password: request.GetPassword(), Source: source})
	if err != nil {
		return nil, ReauthenticationRPCError(err)
	}
	return &identityv1.ReauthenticateWithPasswordResponse{ReauthenticationExpiresAt: timestamppb.New(result.ExpiresAt)}, nil
}
