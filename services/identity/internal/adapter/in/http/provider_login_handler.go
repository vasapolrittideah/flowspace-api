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

var providerNames = map[identityv1.Provider]domain.Provider{
	identityv1.Provider_PROVIDER_GOOGLE: domain.ProviderGoogle,
	identityv1.Provider_PROVIDER_GITHUB: domain.ProviderGitHub,
}

func (h *IdentityHandler) StartProviderLogin(ctx context.Context, request *identityv1.StartProviderLoginRequest) (*identityv1.StartProviderLoginResponse, error) {
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	if len(metadata.ValueFromIncomingContext(ctx, "idempotency-key")) != 0 {
		return nil, invalidSignupArgument("idempotency_key", "is not supported")
	}
	provider, known := providerNames[request.GetProvider()]
	if !known {
		return nil, invalidSignupArgument("provider", "is invalid")
	}
	source, err := h.sourceAddress(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if h.providerLogin == nil {
		return nil, status.Error(codes.Unavailable, "provider login unavailable")
	}
	result, err := h.providerLogin.StartProviderLogin(ctx, inbound.StartProviderLoginInput{Provider: string(provider), Source: source})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidProvider):
			return nil, invalidSignupArgument("provider", "is invalid")
		case errors.Is(err, app.ErrRateLimited):
			return nil, status.Error(codes.ResourceExhausted, "provider login limit exceeded")
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, status.FromContextError(err).Err()
		default:
			return nil, status.Error(codes.Unavailable, "provider login unavailable")
		}
	}
	return &identityv1.StartProviderLoginResponse{
		AuthorizationUrl: result.AuthorizationURL,
		AttemptToken:     result.AttemptToken,
		AttemptExpiresAt: timestamppb.New(result.ExpiresAt),
	}, nil
}

func (h *IdentityHandler) CreateProviderSession(ctx context.Context, request *identityv1.CreateProviderSessionRequest) (*identityv1.CreateProviderSessionResponse, error) {
	return createSession(ctx, h, &identityv1.CreateProviderSessionResponse{}, request == nil, h.providerLogin != nil, "provider login unavailable",
		func(ctx context.Context, source string) (inbound.CreateProviderSessionResult, error) {
			return h.providerLogin.CreateProviderSession(ctx, inbound.CreateProviderSessionInput{
				AttemptToken: request.GetAttemptToken(), HandoffCode: request.GetHandoffCode(), Source: source,
			})
		}, providerSessionRPCError)
}

func providerSessionRPCError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidAttemptToken):
		return invalidSignupArgument("attempt_token", "is invalid")
	case errors.Is(err, domain.ErrInvalidHandoffCode):
		return invalidSignupArgument("handoff_code", "is invalid")
	case errors.Is(err, app.ErrProviderHandoffRejected):
		return status.Error(codes.Unauthenticated, "invalid provider handoff")
	case errors.Is(err, app.ErrProviderAccountUnavailable):
		return status.Error(codes.FailedPrecondition,
			"Unable to complete provider login. Check the provider email or sign in through another method to link the provider.")
	case errors.Is(err, app.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "provider session limit exceeded")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	default:
		return status.Error(codes.Unavailable, "provider login unavailable")
	}
}
