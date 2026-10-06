package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/internal/logging"
	"github.com/vasapolrittideah/flowspace-api/internal/requestid"
	"github.com/vasapolrittideah/flowspace-api/internal/tracing"
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
	propagator := propagation.TraceContext{}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)), grpc.MaxRecvMsgSize(1<<20),
		grpc.UnaryInterceptor(func(ctx context.Context, request any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			if info.FullMethod != identityv1.IdentityService_CheckSession_FullMethodName {
				return nil, status.Error(codes.Unimplemented, "method unavailable")
			}
			started := time.Now()
			incoming, _ := metadata.FromIncomingContext(ctx)
			ctx = propagator.Extract(ctx, tracing.MetadataCarrier(incoming))
			ctx, span := otel.Tracer("flowspace/identity/api").Start(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(semconv.RPCMethod(strings.TrimPrefix(info.FullMethod, "/"))))
			defer span.End()
			response, err := next(ctx, request)
			code := status.Code(err)
			tracing.SetGRPCStatus(span, code)
			checks.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", code.String())))
			logger.Info("identity_session_check", zap.String("request_id", requestid.FromIncoming(ctx)), logging.TraceID(ctx),
				zap.String("operation", info.FullMethod), zap.String("outcome", outcome(code == codes.OK)), zap.String("status", code.String()),
				zap.Duration("duration", time.Since(started)))
			return response, err
		}))
	identityv1.RegisterIdentityServiceServer(server, identitygrpc.NewSessionCheckHandler(service))
	return server, nil
}
