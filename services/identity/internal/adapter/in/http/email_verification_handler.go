package http

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

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
	if h.verifyEmail == nil {
		return nil, status.Error(codes.Unavailable, "email verification unavailable")
	}
	if err := h.verifyEmail.VerifyEmail(ctx, inbound.VerifyEmailInput{
		Subject: identity.Subject, SessionID: identity.SessionID, Source: source, Code: request.GetCode(),
	}); err != nil {
		return nil, verifyEmailRPCError(err)
	}
	return &identityv1.VerifyEmailResponse{EmailVerified: true}, nil
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
