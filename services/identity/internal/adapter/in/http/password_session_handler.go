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
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func (h *IdentityHandler) CreatePasswordSession(ctx context.Context, request *identityv1.CreatePasswordSessionRequest) (*identityv1.CreatePasswordSessionResponse, error) {
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
	if h.passwordLogin == nil {
		return nil, status.Error(codes.Unavailable, "password login unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := h.passwordLogin.CreatePasswordSession(ctx, inbound.CreatePasswordSessionInput{
		Email: request.GetEmail(), Password: request.GetPassword(), Source: source,
	})
	if err != nil {
		return nil, passwordLoginRPCError(err)
	}
	return &identityv1.CreatePasswordSessionResponse{
		Subject: result.Subject, EmailVerified: &result.EmailVerified,
		AccessToken: result.AccessToken, RefreshToken: result.RefreshToken,
		AccessTokenExpiresAt:  timestamppb.New(result.AccessTokenExpiresAt),
		RefreshTokenExpiresAt: timestamppb.New(result.RefreshTokenExpiresAt),
		SessionExpiresAt:      timestamppb.New(result.SessionExpiresAt),
	}, nil
}

func (h *IdentityHandler) RefreshSession(ctx context.Context, request *identityv1.RefreshSessionRequest) (*identityv1.RefreshSessionResponse, error) {
	if len(metadata.ValueFromIncomingContext(ctx, "idempotency-key")) != 0 {
		return nil, invalidSignupArgument("idempotency_key", "is not supported")
	}
	if request == nil || request.GetRefreshToken() == "" {
		return nil, invalidSignupArgument("refresh_token", "is required")
	}
	if h.refresh == nil {
		return nil, status.Error(codes.Unavailable, "refresh unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := h.refresh.RefreshSession(ctx, request.GetRefreshToken())
	if err != nil {
		return nil, refreshRPCError(err)
	}
	return &identityv1.RefreshSessionResponse{
		AccessToken: result.AccessToken, RefreshToken: result.RefreshToken,
		AccessTokenExpiresAt:  timestamppb.New(result.AccessTokenExpiresAt),
		RefreshTokenExpiresAt: timestamppb.New(result.RefreshTokenExpiresAt),
		SessionExpiresAt:      timestamppb.New(result.SessionExpiresAt),
	}, nil
}

func (h *IdentityHandler) LogoutCurrentSession(ctx context.Context, request *identityv1.LogoutCurrentSessionRequest) (*identityv1.LogoutCurrentSessionResponse, error) {
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	identity, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if h.currentLogout == nil {
		return nil, status.Error(codes.Unavailable, "logout unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := h.currentLogout.LogoutCurrentSession(ctx, inbound.LogoutCurrentSessionInput{Subject: identity.Subject, SessionID: identity.SessionID}); err != nil {
		return nil, logoutRPCError(err)
	}
	return &identityv1.LogoutCurrentSessionResponse{Revoked: true}, nil
}

func (h *IdentityHandler) LogoutAllSessions(ctx context.Context, request *identityv1.LogoutAllSessionsRequest) (*identityv1.LogoutAllSessionsResponse, error) {
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	identity, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if h.allLogout == nil {
		return nil, status.Error(codes.Unavailable, "logout unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := h.allLogout.LogoutAllSessions(ctx, inbound.LogoutAllSessionsInput{Subject: identity.Subject, SessionID: identity.SessionID}); err != nil {
		return nil, logoutRPCError(err)
	}
	return &identityv1.LogoutAllSessionsResponse{Revoked: true}, nil
}

func passwordLoginRPCError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidEmail):
		return invalidSignupArgument("email", "is invalid")
	case errors.Is(err, domain.ErrInvalidPassword):
		return invalidSignupArgument("password", "is invalid")
	case errors.Is(err, app.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "password login limit exceeded")
	case errors.Is(err, app.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, "invalid credentials")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	default:
		return status.Error(codes.Unavailable, "password login unavailable")
	}
}

func refreshRPCError(err error) error {
	switch {
	case errors.Is(err, app.ErrInvalidRefreshToken):
		return invalidSignupArgument("refresh_token", "is invalid")
	case errors.Is(err, app.ErrUnauthenticatedRefresh):
		return status.Error(codes.Unauthenticated, "invalid refresh token")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	default:
		return status.Error(codes.Unavailable, "refresh unavailable")
	}
}

func logoutRPCError(err error) error {
	switch {
	case errors.Is(err, outbound.ErrUnauthenticated):
		return status.Error(codes.Unauthenticated, "authentication required")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	default:
		return status.Error(codes.Unavailable, "logout unavailable")
	}
}
