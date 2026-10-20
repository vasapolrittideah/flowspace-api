package http

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

func TestReauthenticationRPCError(t *testing.T) {
	if err := ReauthenticationRPCError(nil); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		err  error
		code codes.Code
	}{
		{app.ErrReauthenticationRequired, codes.PermissionDenied},
		{outbound.ErrUnauthenticated, codes.Unauthenticated},
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
		{errors.New("private database detail"), codes.Unavailable},
	} {
		if got := ReauthenticationRPCError(test.err); status.Code(got) != test.code || got.Error() == test.err.Error() {
			t.Fatalf("RPC error = %v, want %v", got, test.code)
		}
	}
	private := "private database detail"
	if got := ReauthenticationRPCError(errors.New(private)); status.Convert(got).Message() != "re-authentication unavailable" || strings.Contains(got.Error(), private) || len(status.Convert(got).Details()) != 0 {
		t.Fatalf("private detail escaped: %v", got)
	}
	result := status.Convert(ReauthenticationRPCError(app.ErrReauthenticationRequired))
	if len(result.Details()) != 1 {
		t.Fatalf("details = %v", result.Details())
	}
	detail, ok := result.Details()[0].(*errdetails.ErrorInfo)
	if !ok || detail.GetReason() != "REAUTHENTICATION_REQUIRED" || detail.GetDomain() != "identity.flowspace" {
		t.Fatalf("error detail = %v", detail)
	}
}
