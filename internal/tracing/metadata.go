package tracing

import (
	"maps"
	"slices"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc/metadata"
)

// MetadataCarrier lets a propagator read and write the trace context in gRPC
// metadata. Get ignores a key that has more than one value, so a caller
// cannot choose between two parents.
type MetadataCarrier metadata.MD

var _ propagation.TextMapCarrier = MetadataCarrier(nil)

func (c MetadataCarrier) Get(key string) string {
	values := metadata.MD(c).Get(key)
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func (c MetadataCarrier) Set(key, value string) {
	metadata.MD(c).Set(key, value)
}

func (c MetadataCarrier) Keys() []string {
	return slices.Collect(maps.Keys(c))
}
