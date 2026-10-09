package tracing

import (
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
)

// grpcStatusCodeKey is the span attribute that the specification names.
// Newer semantic conventions call it rpc.response.status_code.
const grpcStatusCodeKey = attribute.Key("rpc.grpc.status_code")

// HTTPMethod returns method when it is a standard HTTP method, or _OTHER,
// so that a client cannot add new span names or attribute values.
func HTTPMethod(method string) string {
	switch method {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace:
		return method
	}
	return "_OTHER"
}

// SetHTTPStatus records the HTTP status of a server span. A 5xx status sets
// the error status.
func SetHTTPStatus(span trace.Span, status int) {
	span.SetAttributes(semconv.HTTPResponseStatusCode(status))
	if status >= http.StatusInternalServerError {
		span.SetStatus(otelcodes.Error, "")
	}
}

// GRPCStatusCode returns the rpc.grpc.status_code attribute of code.
func GRPCStatusCode(code codes.Code) attribute.KeyValue {
	return grpcStatusCodeKey.Int64(int64(code))
}

// SetGRPCStatus records the gRPC status code of a server span. A code that
// shows a server failure sets the error status.
func SetGRPCStatus(span trace.Span, code codes.Code) {
	span.SetAttributes(GRPCStatusCode(code))
	if code == codes.Internal || code == codes.Unavailable || code == codes.DeadlineExceeded || code == codes.Unknown {
		span.SetStatus(otelcodes.Error, "")
	}
}

// GRPCStatus reads the gRPC status code that a gRPC server wrote to the
// Grpc-Status trailer of an HTTP response.
func GRPCStatus(header http.Header) (codes.Code, bool) {
	code, err := strconv.ParseUint(header.Get("Grpc-Status"), 10, 32)
	if err != nil || code > uint64(codes.Unauthenticated) {
		return 0, false
	}
	return codes.Code(code), true
}
