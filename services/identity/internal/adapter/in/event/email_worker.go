package event

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var (
	ErrInvalidDeliveryEvent = errors.New("invalid delivery event")
	ErrEmailDelivery        = errors.New("email delivery failed")
	ErrDeliveryBroker       = errors.New("delivery broker failed")
)

type EmailWorker struct {
	client     *kgo.Client
	repository outbound.DeliveryRepository
	opener     outbound.DeliveryOpener
	sender     outbound.EmailSender
	logger     *zap.Logger
}

func NewEmailWorker(broker, topic, group string, repository outbound.DeliveryRepository, opener outbound.DeliveryOpener, sender outbound.EmailSender, logger *zap.Logger) (*EmailWorker, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(broker), kgo.ConsumeTopics(topic), kgo.ConsumerGroup(group),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll(),
	)
	if err != nil {
		return nil, err
	}
	return &EmailWorker{client: client, repository: repository, opener: opener, sender: sender, logger: logger}, nil
}

func (w *EmailWorker) Close() { w.client.Close() }

func (w *EmailWorker) HandleRecord(ctx context.Context, record *kgo.Record) error {
	if record == nil {
		return ErrInvalidDeliveryEvent
	}
	header := &sr.ConfluentHeader{}
	schemaID, payload, err := header.DecodeID(record.Value)
	if err != nil || schemaID < 1 {
		return ErrInvalidDeliveryEvent
	}
	_, payload, err = header.DecodeIndex(payload, 1)
	if err != nil {
		return ErrInvalidDeliveryEvent
	}
	var request identityv1.EmailDeliveryRequested
	if err := proto.Unmarshal(payload, &request); err != nil {
		return ErrInvalidDeliveryEvent
	}
	if _, err := uuid.Parse(request.GetEventId()); err != nil {
		return ErrInvalidDeliveryEvent
	}
	if _, err := uuid.Parse(request.GetChallengeId()); err != nil || string(record.Key) != request.GetEventId() ||
		(request.GetPurpose() != string(domain.PurposeVerifyEmail) && request.GetPurpose() != string(domain.PurposeClaimAccount)) {
		return ErrInvalidDeliveryEvent
	}
	if err := w.repository.WithCurrentDelivery(ctx, request.GetChallengeId(), request.GetPurpose(), func(ctx context.Context, current outbound.CurrentDelivery) error {
		address, code, err := w.opener.Open(request.GetChallengeId(), request.GetPurpose(), current.Subject, current.Material)
		if err != nil || address != current.Email {
			return ErrEmailDelivery
		}
		if err := w.sender.Send(ctx, address, code, request.GetPurpose()); err != nil {
			return ErrEmailDelivery
		}
		return nil
	}); err != nil {
		return ErrEmailDelivery
	}
	return nil
}

func (w *EmailWorker) RunOnce(ctx context.Context) (bool, error) {
	fetches := w.client.PollRecords(ctx, 1)
	defer w.client.AllowRebalance()
	if len(fetches.Errors()) > 0 {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		return false, ErrDeliveryBroker
	}
	records := fetches.Records()
	if len(records) == 0 {
		return false, nil
	}
	carrier := propagation.MapCarrier{}
	for _, header := range records[0].Headers {
		if header.Key == "traceparent" || header.Key == "tracestate" {
			carrier.Set(header.Key, string(header.Value))
		}
	}
	ctx = (propagation.TraceContext{}).Extract(ctx, carrier)
	ctx, span := otel.Tracer("flowspace/identity/email-worker").Start(ctx, "identity.email_delivery")
	defer span.End()
	if err := w.HandleRecord(ctx, records[0]); err != nil {
		span.SetStatus(codes.Error, "delivery failed")
		w.logger.Warn("email_delivery_failed", zap.String("trace_id", trace.SpanContextFromContext(ctx).TraceID().String()))
		return true, err
	}
	if err := w.client.CommitRecords(ctx, records[0]); err != nil {
		span.SetStatus(codes.Error, "broker commit failed")
		w.logger.Warn("email_delivery_commit_failed", zap.String("trace_id", trace.SpanContextFromContext(ctx).TraceID().String()))
		return true, ErrDeliveryBroker
	}
	if !records[0].Timestamp.IsZero() {
		w.logger.Info("email_delivery_processed", zap.String("trace_id", trace.SpanContextFromContext(ctx).TraceID().String()),
			zap.Float64("record_age_seconds", max(0, time.Since(records[0].Timestamp).Seconds())))
	} else {
		w.logger.Info("email_delivery_processed", zap.String("trace_id", trace.SpanContextFromContext(ctx).TraceID().String()))
	}
	return true, nil
}

func (w *EmailWorker) Run(ctx context.Context) error {
	nextCleanup := time.Now()
	for {
		if !time.Now().Before(nextCleanup) {
			if err := w.repository.PurgeTerminal(ctx); err != nil {
				return ErrEmailDelivery
			}
			nextCleanup = time.Now().Add(time.Minute)
		}
		pollCtx, cancel := context.WithDeadline(ctx, nextCleanup)
		_, err := w.RunOnce(pollCtx)
		cancel()
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			continue
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}
