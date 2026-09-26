package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identitygrpc "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/grpc"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
)

func newSessionGRPCServer(tlsConfig *tls.Config, service inbound.SessionCheckService, meter metric.Meter, logger *zap.Logger) (*grpc.Server, error) {
	if tlsConfig == nil || service == nil {
		return nil, errors.New("session RPC unavailable")
	}
	checks, err := meter.Int64Counter("identity.session_checks",
		metric.WithDescription("Identity session checks by gRPC outcome"))
	if err != nil {
		return nil, errors.New("session metrics unavailable")
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)), grpc.MaxRecvMsgSize(1<<20),
		grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			if info.FullMethod != identityv1.IdentityService_CheckSession_FullMethodName {
				return nil, status.Error(codes.Unimplemented, "method unavailable")
			}
			started := time.Now()
			response, err := next(ctx, request)
			outcome := status.Code(err).String()
			checks.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
			logger.Info("identity_session_check", zap.String("outcome", outcome), zap.Duration("duration", time.Since(started)))
			return response, err
		}))
	identityv1.RegisterIdentityServiceServer(server, identitygrpc.NewSessionCheckHandler(service))
	return server, nil
}
