package event

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sr"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/protobuf/proto"

	"github.com/vasapolrittideah/flowspace-api/contracts/events"
	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

type Publisher struct {
	client   *kgo.Client
	topic    string
	schemaID int
}

func NewPublisher(ctx context.Context, client *kgo.Client, registry *sr.Client, topic string) (*Publisher, error) {
	schema, err := registry.CreateSchema(ctx, topic+"-value", sr.Schema{
		Schema: events.EmailDeliveryRequestedSchema, Type: sr.TypeProtobuf,
	})
	if err != nil {
		return nil, err
	}
	return &Publisher{client: client, topic: topic, schemaID: schema.ID}, nil
}

func (p *Publisher) Publish(ctx context.Context, request outbound.OutboxEvent) error {
	payload, err := proto.Marshal(&identityv1.EmailDeliveryRequested{
		EventId: request.ID, ChallengeId: request.ChallengeID, Purpose: request.Purpose,
	})
	if err != nil {
		return err
	}
	header, err := (&sr.ConfluentHeader{}).AppendEncode(nil, p.schemaID, []int{0})
	if err != nil {
		return err
	}
	carrier := propagation.MapCarrier{}
	(propagation.TraceContext{}).Inject(ctx, carrier)
	record := &kgo.Record{Topic: p.topic, Key: []byte(request.ID), Value: append(header, payload...)}
	for name, value := range carrier {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: name, Value: []byte(value)})
	}
	return p.client.ProduceSync(ctx, record).FirstErr()
}
