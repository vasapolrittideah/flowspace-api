package http

import (
	"context"
	"errors"

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
