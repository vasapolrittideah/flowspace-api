package http

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func (h *IdentityHandler) CreatePasswordSession(ctx context.Context, request *identityv1.CreatePasswordSessionRequest) (*identityv1.CreatePasswordSessionResponse, error) {
	return createSession(ctx, h, &identityv1.CreatePasswordSessionResponse{}, request == nil, h.passwordLogin != nil, "password login unavailable",
		func(ctx context.Context, source string) (inbound.CreatePasswordSessionResult, error) {
			return h.passwordLogin.CreatePasswordSession(ctx, inbound.CreatePasswordSessionInput{
				Email: request.GetEmail(), Password: request.GetPassword(), Source: source,
			})
		}, passwordLoginRPCError)
}

// createSession applies the shared checks of an unauthenticated
// session-creating RPC. It calls create with the source address under a
// five-second deadline, maps its error with rpcError, and fills response.
func createSession[T proto.Message](ctx context.Context, h *IdentityHandler, response T, missing, available bool, unavailable string,
	create func(context.Context, string) (inbound.CreatePasswordSessionResult, error), rpcError func(error) error,
) (T, error) {
	var none T
	if len(metadata.ValueFromIncomingContext(ctx, "idempotency-key")) != 0 {
		return none, invalidSignupArgument("idempotency_key", "is not supported")
	}
	if missing {
		return none, invalidSignupArgument("request", "is required")
	}
	source, err := h.sourceAddress(ctx)
	if err != nil {
		return none, err
	}
	if !available {
		return none, status.Error(codes.Unavailable, unavailable)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := create(ctx, source)
	if err != nil {
		return none, rpcError(err)
	}
	// Password and provider session responses share these field names.
	message := response.ProtoReflect()
	fields := message.Descriptor().Fields()
	for name, value := range map[protoreflect.Name]protoreflect.Value{
		"subject":                  protoreflect.ValueOfString(result.Subject),
		"email_verified":           protoreflect.ValueOfBool(result.EmailVerified),
		"access_token":             protoreflect.ValueOfString(result.AccessToken),
		"refresh_token":            protoreflect.ValueOfString(result.RefreshToken),
		"access_token_expires_at":  protoreflect.ValueOfMessage(timestamppb.New(result.AccessTokenExpiresAt).ProtoReflect()),
		"refresh_token_expires_at": protoreflect.ValueOfMessage(timestamppb.New(result.RefreshTokenExpiresAt).ProtoReflect()),
		"session_expires_at":       protoreflect.ValueOfMessage(timestamppb.New(result.SessionExpiresAt).ProtoReflect()),
	} {
		message.Set(fields.ByName(name), value)
	}
	return response, nil
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
