package event

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/vasapolrittideah/flowspace-api/internal/logging"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

const publishOperation = "identity.outbox_publish"

type OutboxRelay struct {
	repository outbound.OutboxRepository
	publish    func(context.Context, outbound.OutboxEvent) error
	owner      string
	logger     *zap.Logger
}

func NewOutboxRelay(repository outbound.OutboxRepository, publish func(context.Context, outbound.OutboxEvent) error, owner string,
	logger *zap.Logger,
) *OutboxRelay {
	return &OutboxRelay{repository: repository, publish: publish, owner: owner, logger: logger}
}

// RunOnce publishes one claimed event in an identity.outbox_publish span. The
// span continues the trace context that the outbox stored with the event, or
// starts a new root trace when the event has no valid stored context.
func (r *OutboxRelay) RunOnce(ctx context.Context) (bool, error) {
	request, ok, err := r.repository.Claim(ctx, r.owner)
	if err != nil {
		r.logger.Warn("outbox_publish_failed", zap.String("operation", publishOperation))
		return false, err
	}
	if !ok {
		return false, nil
	}
	// Drop the span of the caller, so that an invalid stored context starts a
	// new root.
	ctx = propagation.TraceContext{}.Extract(trace.ContextWithSpanContext(ctx, trace.SpanContext{}),
		propagation.MapCarrier{"traceparent": request.Traceparent, "tracestate": request.Tracestate})
	ctx, span := otel.Tracer("flowspace/identity/outbox-relay").Start(ctx, publishOperation, trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()
	if err := r.publish(ctx, request); err != nil {
		err = errors.Join(err, r.repository.Release(ctx, request.ID, r.owner, time.Now().Add(time.Second)))
		r.fail(ctx, span)
		return true, err
	}
	if err := r.repository.MarkPublished(ctx, request.ID, r.owner); err != nil {
		r.fail(ctx, span)
		return true, err
	}
	return true, nil
}

func (r *OutboxRelay) fail(ctx context.Context, span trace.Span) {
	span.SetStatus(codes.Error, "")
	r.logger.Warn("outbox_publish_failed", logging.TraceID(ctx), zap.String("operation", publishOperation))
}
