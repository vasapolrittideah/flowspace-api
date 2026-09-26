package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type SessionCheckHandler struct {
	identityv1.UnimplementedIdentityServiceServer
	service inbound.SessionCheckService
}

var _ identityv1.IdentityServiceServer = (*SessionCheckHandler)(nil)

func NewSessionCheckHandler(service inbound.SessionCheckService) *SessionCheckHandler {
	return &SessionCheckHandler{service: service}
}

func (h *SessionCheckHandler) CheckSession(ctx context.Context, request *identityv1.CheckSessionRequest) (*identityv1.CheckSessionResponse, error) {
	if request == nil || request.GetSubject() == "" || request.GetSessionId() == "" {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}
	if h.service == nil {
		return nil, status.Error(codes.Unavailable, "session check unavailable")
	}
	verified, err := h.service.CheckSession(ctx, inbound.CheckSessionInput{Subject: request.GetSubject(), SessionID: request.GetSessionId()})
	if err != nil {
		switch {
		case errors.Is(err, outbound.ErrUnauthenticated):
			return nil, status.Error(codes.Unauthenticated, "authentication required")
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return nil, status.FromContextError(err).Err()
		default:
			return nil, status.Error(codes.Unavailable, "session check unavailable")
		}
	}
	return &identityv1.CheckSessionResponse{EmailVerified: verified}, nil
}
