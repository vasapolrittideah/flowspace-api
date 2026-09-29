package http

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

func (h *IdentityHandler) RequestPasswordResetCode(ctx context.Context, request *identityv1.RequestPasswordResetCodeRequest) (*identityv1.RequestPasswordResetCodeResponse, error) {
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	source, err := h.sourceAddress(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if h.passwordRecovery == nil {
		return nil, status.Error(codes.Unavailable, "password recovery unavailable")
	}
	if err := h.passwordRecovery.RequestPasswordResetCode(ctx, inbound.RequestPasswordResetCodeInput{Email: request.GetEmail(), Source: source}); err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidEmail):
			return nil, invalidSignupArgument("email", "is invalid")
		case errors.Is(err, app.ErrRateLimited):
			return nil, status.Error(codes.ResourceExhausted, "recovery request limit exceeded")
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, status.FromContextError(err).Err()
		default:
			return nil, status.Error(codes.Unavailable, "password recovery unavailable")
		}
	}
	return &identityv1.RequestPasswordResetCodeResponse{Accepted: true}, nil
}
