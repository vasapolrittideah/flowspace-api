package http

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

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
