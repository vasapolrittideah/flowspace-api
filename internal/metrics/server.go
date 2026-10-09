package metrics

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"google.golang.org/grpc/codes"

	"github.com/vasapolrittideah/flowspace-api/internal/tracing"
)

// unmatchedRoute and unknownMethod replace a raw path and the name of a gRPC
// method that the server does not register, so that labels stay bounded.
const (
	unmatchedRoute = "unmatched"
	unknownMethod  = "unknown"
)

// ServerDurations records the duration of each public HTTP request and gRPC
// call with the bounded method, route, and status values of its server span.
type ServerDurations struct {
	http metric.Float64Histogram
	rpc  metric.Float64Histogram
}

// NewServerDurations creates the http.server.request.duration and
// rpc.server.call.duration histograms. An instrument error goes to the
// global error handler, because telemetry never stops a process.
func NewServerDurations(meter metric.Meter) ServerDurations {
	httpDuration, httpErr := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"),
		metric.WithDescription("Duration of each public HTTP request"))
	rpcDuration, rpcErr := meter.Float64Histogram("rpc.server.call.duration", metric.WithUnit("s"),
		metric.WithDescription("Duration of each public gRPC call"))
	if err := errors.Join(httpErr, rpcErr); err != nil {
		otel.Handle(err)
	}
	return ServerDurations{http: httpDuration, rpc: rpcDuration}
}

// RecordHTTP records an HTTP request. route is the template of the matched
// route, or empty when no route matches.
func (d ServerDurations) RecordHTTP(ctx context.Context, elapsed time.Duration, method, route string, status int) {
	if route == "" {
		route = unmatchedRoute
	}
	d.http.Record(ctx, elapsed.Seconds(), metric.WithAttributes(
		semconv.HTTPRequestMethodKey.String(tracing.HTTPMethod(method)), semconv.HTTPRoute(route), semconv.HTTPResponseStatusCode(status)))
}

// RecordRPC records a gRPC call. method is the full name of a registered
// method, or empty for another method.
func (d ServerDurations) RecordRPC(ctx context.Context, elapsed time.Duration, method string, code codes.Code) {
	name := strings.TrimPrefix(method, "/")
	if name == "" {
		name = unknownMethod
	}
	d.rpc.Record(ctx, elapsed.Seconds(), metric.WithAttributes(semconv.RPCMethod(name), tracing.GRPCStatusCode(code)))
}
