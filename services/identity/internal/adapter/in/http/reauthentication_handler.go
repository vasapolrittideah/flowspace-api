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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func ReauthenticationRPCError(err error) error {
	if err == nil {
		return nil
	}
	if invalid := reauthenticationArgumentError(err); invalid != nil {
		return invalid
	}
	switch {
	case errors.Is(err, app.ErrReauthenticationEmailUnverified):
		return status.Error(codes.FailedPrecondition, "verified email required")
	case errors.Is(err, app.ErrProviderHandoffRejected), errors.Is(err, app.ErrProviderAccountUnavailable):
		return status.Error(codes.FailedPrecondition, "provider re-authentication failed")
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

// reauthenticationArgumentError returns the InvalidArgument error of a
// rejected proof, or nil for any other error.
func reauthenticationArgumentError(err error) error {
	for _, field := range []struct {
		err  error
		name string
	}{
		{app.ErrInvalidReauthenticationCode, "code"},
		{domain.ErrInvalidPassword, "password"},
		{domain.ErrInvalidProvider, "provider"},
		{domain.ErrInvalidAttemptToken, "attempt_token"},
		{domain.ErrInvalidHandoffCode, "handoff_code"},
	} {
		if errors.Is(err, field.err) {
			return invalidSignupArgument(field.name, "is invalid")
		}
	}
	if errors.Is(err, app.ErrInvalidCredentials) {
		return status.Error(codes.InvalidArgument, "invalid credentials")
	}
	return nil
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

func (h *IdentityHandler) RequestReauthenticationCode(ctx context.Context, request *identityv1.RequestReauthenticationCodeRequest) (*identityv1.RequestReauthenticationCodeResponse, error) {
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
	if h.codeReauthentication == nil {
		return nil, ReauthenticationRPCError(app.ErrReauthenticationUnavailable)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := h.codeReauthentication.RequestReauthenticationCode(ctx, inbound.RequestReauthenticationCodeInput{Subject: identity.Subject, SessionID: identity.SessionID, Source: source}); err != nil {
		return nil, ReauthenticationRPCError(err)
	}
	return &identityv1.RequestReauthenticationCodeResponse{Accepted: true}, nil
}

func (h *IdentityHandler) ReauthenticateWithCode(ctx context.Context, request *identityv1.ReauthenticateWithCodeRequest) (*identityv1.ReauthenticateWithCodeResponse, error) {
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
	if h.codeReauthentication == nil {
		return nil, ReauthenticationRPCError(app.ErrReauthenticationUnavailable)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := h.codeReauthentication.ReauthenticateWithCode(ctx, inbound.ReauthenticateWithCodeInput{Subject: identity.Subject, SessionID: identity.SessionID, Source: source, Code: request.GetCode()})
	if err != nil {
		return nil, ReauthenticationRPCError(err)
	}
	return &identityv1.ReauthenticateWithCodeResponse{ReauthenticationExpiresAt: timestamppb.New(result.ExpiresAt)}, nil
}

func (h *IdentityHandler) StartProviderReauthentication(ctx context.Context, request *identityv1.StartProviderReauthenticationRequest) (*identityv1.StartProviderReauthenticationResponse, error) {
	if request == nil {
		return nil, invalidSignupArgument("request", "is required")
	}
	identity, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
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
	if h.providerReauthentication == nil {
		return nil, ReauthenticationRPCError(app.ErrReauthenticationUnavailable)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := h.providerReauthentication.StartProviderReauthentication(ctx, inbound.StartProviderReauthenticationInput{
		Subject: identity.Subject, SessionID: identity.SessionID, Provider: string(provider), Source: source,
	})
	if err != nil {
		return nil, ReauthenticationRPCError(err)
	}
	return &identityv1.StartProviderReauthenticationResponse{
		AuthorizationUrl: result.AuthorizationURL, AttemptToken: result.AttemptToken, AttemptExpiresAt: timestamppb.New(result.ExpiresAt),
	}, nil
}

func (h *IdentityHandler) CompleteProviderReauthentication(ctx context.Context, request *identityv1.CompleteProviderReauthenticationRequest) (*identityv1.CompleteProviderReauthenticationResponse, error) {
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
	if h.providerReauthentication == nil {
		return nil, ReauthenticationRPCError(app.ErrReauthenticationUnavailable)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := h.providerReauthentication.CompleteProviderReauthentication(ctx, inbound.CompleteProviderReauthenticationInput{
		Subject: identity.Subject, SessionID: identity.SessionID, AttemptToken: request.GetAttemptToken(), HandoffCode: request.GetHandoffCode(), Source: source,
	})
	if err != nil {
		return nil, ReauthenticationRPCError(err)
	}
	return &identityv1.CompleteProviderReauthenticationResponse{ReauthenticationExpiresAt: timestamppb.New(result.ExpiresAt)}, nil
}
