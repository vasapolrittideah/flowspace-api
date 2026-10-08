package event

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	"github.com/vasapolrittideah/flowspace-api/internal/logging"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

const (
	deliveryOperation = "identity.email_delivery"
	noticeOperation   = "identity.password_change_notice"
	noticeKind        = "password-change-notice"
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
	deliveries metric.Int64Counter
}

func NewEmailWorker(broker, topic, group string, repository outbound.DeliveryRepository, opener outbound.DeliveryOpener, sender outbound.EmailSender, logger *zap.Logger, options ...kgo.Opt) (*EmailWorker, error) {
	client, err := kgo.NewClient(append([]kgo.Opt{
		kgo.SeedBrokers(broker), kgo.ConsumeTopics(topic), kgo.ConsumerGroup(group),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.DisableAutoCommit(), kgo.BlockRebalanceOnPoll(),
	}, options...)...)
	if err != nil {
		return nil, err
	}
	deliveries, err := otel.Meter("flowspace/identity/email-worker").Int64Counter("identity.email.deliveries",
		metric.WithUnit("{delivery}"), metric.WithDescription("Email delivery attempts by kind and outcome"))
	if err != nil {
		client.Close()
		return nil, err
	}
	return &EmailWorker{client: client, repository: repository, opener: opener, sender: sender, logger: logger, deliveries: deliveries}, nil
}

func (w *EmailWorker) Close() { w.client.Close() }

func (w *EmailWorker) HandleRecord(ctx context.Context, record *kgo.Record) error {
	_, err := w.handleRecord(ctx, record)
	return err
}

// handleRecord returns the code purpose when it tries a current delivery, or
// an empty kind when the record sends no email.
func (w *EmailWorker) handleRecord(ctx context.Context, record *kgo.Record) (string, error) {
	if record == nil {
		return "", ErrInvalidDeliveryEvent
	}
	header := &sr.ConfluentHeader{}
	schemaID, payload, err := header.DecodeID(record.Value)
	if err != nil || schemaID < 1 {
		return "", ErrInvalidDeliveryEvent
	}
	_, payload, err = header.DecodeIndex(payload, 1)
	if err != nil {
		return "", ErrInvalidDeliveryEvent
	}
	var request identityv1.EmailDeliveryRequested
	if err := proto.Unmarshal(payload, &request); err != nil {
		return "", ErrInvalidDeliveryEvent
	}
	if _, err := uuid.Parse(request.GetEventId()); err != nil {
		return "", ErrInvalidDeliveryEvent
	}
	if _, err := uuid.Parse(request.GetChallengeId()); err != nil || string(record.Key) != request.GetEventId() ||
		!validDeliveryPurpose(request.GetPurpose()) {
		return "", ErrInvalidDeliveryEvent
	}
	kind := ""
	if err := w.repository.WithCurrentDelivery(ctx, request.GetChallengeId(), request.GetPurpose(), func(ctx context.Context, current outbound.CurrentDelivery) error {
		kind = request.GetPurpose()
		address, code, err := w.opener.Open(request.GetChallengeId(), request.GetPurpose(), current.Subject, current.Material)
		if err != nil || address != current.Email {
			return ErrEmailDelivery
		}
		if err := w.sender.Send(ctx, address, code, request.GetPurpose()); err != nil {
			return ErrEmailDelivery
		}
		return nil
	}); err != nil {
		return kind, ErrEmailDelivery
	}
	return kind, nil
}

// DeliverPasswordChangeNotice sends one due notice and reports whether it found one.
func (w *EmailWorker) DeliverPasswordChangeNotice(ctx context.Context) (bool, error) {
	// A notice comes from a table row and not from an event record, so it
	// starts a new root trace.
	ctx, span := otel.Tracer("flowspace/identity/email-worker").Start(ctx, noticeOperation,
		trace.WithSpanKind(trace.SpanKindConsumer), trace.WithNewRoot())
	defer span.End()
	attempted := false
	found, err := w.repository.WithNextPasswordChangeNotice(ctx, func(ctx context.Context, email string) error {
		attempted = true
		return w.sender.SendPasswordChangeNotice(ctx, email)
	})
	if err != nil {
		if attempted {
			w.countDelivery(ctx, noticeKind, "failed")
		}
		span.SetStatus(codes.Error, "notice delivery failed")
		w.logger.Warn("password_change_notice_failed", logging.TraceID(ctx), zap.String("operation", noticeOperation))
		return found, ErrEmailDelivery
	}
	if attempted {
		w.countDelivery(ctx, noticeKind, "delivered")
	}
	return found, nil
}

// countDelivery adds one delivery attempt. An empty kind means that the worker did not try to send an email.
func (w *EmailWorker) countDelivery(ctx context.Context, kind, outcome string) {
	if kind != "" {
		w.deliveries.Add(ctx, 1, metric.WithAttributes(attribute.String("identity.email.kind", kind), attribute.String("outcome", outcome)))
	}
}

func validDeliveryPurpose(purpose string) bool {
	switch purpose {
	case string(domain.PurposeVerifyEmail), string(domain.PurposeClaimAccount), string(domain.PurposePasswordReset):
		return true
	default:
		return false
	}
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
	return true, w.processRecord(ctx, records[0], w.client.CommitRecords)
}

func (w *EmailWorker) processRecord(ctx context.Context, record *kgo.Record, commit func(context.Context, ...*kgo.Record) error) error {
	carrier := propagation.MapCarrier{}
	for _, header := range record.Headers {
		if header.Key == "traceparent" || header.Key == "tracestate" {
			carrier.Set(header.Key, string(header.Value))
		}
	}
	ctx = (propagation.TraceContext{}).Extract(ctx, carrier)
	ctx, span := otel.Tracer("flowspace/identity/email-worker").Start(ctx, deliveryOperation, trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End()
	traceID, operation := logging.TraceID(ctx), zap.String("operation", deliveryOperation)
	kind, err := w.handleRecord(ctx, record)
	if err != nil {
		w.countDelivery(ctx, kind, "failed")
		w.client.SetOffsets(map[string]map[int32]kgo.EpochOffset{
			record.Topic: {record.Partition: {Epoch: record.LeaderEpoch, Offset: record.Offset}},
		})
		span.SetStatus(codes.Error, "delivery failed")
		w.logger.Warn("email_delivery_failed", traceID, operation)
		return err
	}
	if err := commit(ctx, record); err != nil {
		w.countDelivery(ctx, kind, "commit_failed")
		span.SetStatus(codes.Error, "broker commit failed")
		w.logger.Warn("email_delivery_commit_failed", traceID, operation)
		return ErrDeliveryBroker
	}
	if !record.Timestamp.IsZero() {
		w.logger.Info("email_delivery_processed", traceID, operation,
			zap.Float64("record_age_seconds", max(0, time.Since(record.Timestamp).Seconds())))
	} else {
		w.logger.Info("email_delivery_processed", traceID, operation)
	}
	w.countDelivery(ctx, kind, "delivered")
	return nil
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
