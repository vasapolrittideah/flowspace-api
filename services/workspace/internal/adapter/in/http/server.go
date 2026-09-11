package httptransport

import (
	"context"
	"net/http"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	workspacev1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/workspace/v1"
	"google.golang.org/grpc"
)

func NewServerHandler(ctx context.Context, handler *WorkspaceHandler) (http.Handler, error) {
	grpcServer := grpc.NewServer()
	workspacev1.RegisterWorkspaceServiceServer(grpcServer, handler)

	gateway := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(incomingHeader))
	if err := workspacev1.RegisterWorkspaceServiceHandlerServer(ctx, gateway, handler); err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.ProtoMajor == 2 && strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/grpc") {
			grpcServer.ServeHTTP(response, request)
			return
		}
		gateway.ServeHTTP(response, request)
	}), nil
}

func incomingHeader(key string) (string, bool) {
	if strings.EqualFold(key, "Idempotency-Key") {
		return "idempotency-key", true
	}
	return runtime.DefaultHeaderMatcher(key)
}
