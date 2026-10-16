package messaging

import (
	"context"
	"errors"
	"testing"

	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
)

type fakePublisherChannel struct {
	exchangeName    string
	exchangeKind    string
	exchangeDurable bool

	publishExchange string
	routingKey      string
	published       amqp.Publishing
	publishErr      error
}

func (f *fakePublisherChannel) ExchangeDeclare(
	name, kind string,
	durable, autoDelete, internal, noWait bool,
	args amqp.Table,
) error {
	f.exchangeName = name
	f.exchangeKind = kind
	f.exchangeDurable = durable
	return nil
}

func (f *fakePublisherChannel) PublishWithContext(
	ctx context.Context,
	exchange, key string,
	mandatory, immediate bool,
	msg amqp.Publishing,
) error {
	f.publishExchange = exchange
	f.routingKey = key
	f.published = msg
	return f.publishErr
}

func TestNewPublisherDeclaresDurableTopicExchange(t *testing.T) {
	channel := &fakePublisherChannel{}

	if _, err := NewPublisher(channel); err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}

	if channel.exchangeName != ExchangeName {
		t.Errorf("exchange = %q, want %q", channel.exchangeName, ExchangeName)
	}
	if channel.exchangeKind != "topic" {
		t.Errorf("exchange kind = %q, want topic", channel.exchangeKind)
	}
	if !channel.exchangeDurable {
		t.Error("exchange is not durable")
	}
}

func TestPublishSetsEventIDAndSendsPersistentProtobuf(t *testing.T) {
	channel := &fakePublisherChannel{}
	publisher, err := NewPublisher(channel)
	if err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}

	event := &eventsv1.HoldRequested{
		Metadata: &eventsv1.EventMetadata{CorrelationId: "saga-123"},
		HoldId:   "hold-123",
	}
	if err := publisher.Publish(context.Background(), "hold.requested", event); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if event.GetMetadata().GetEventId() == "" {
		t.Fatal("Publish() did not set metadata.event_id")
	}
	if channel.publishExchange != ExchangeName {
		t.Errorf("published exchange = %q, want %q", channel.publishExchange, ExchangeName)
	}
	if channel.routingKey != "hold.requested" {
		t.Errorf("routing key = %q, want hold.requested", channel.routingKey)
	}
	if channel.published.DeliveryMode != amqp.Persistent {
		t.Errorf("delivery mode = %d, want persistent", channel.published.DeliveryMode)
	}
	if channel.published.ContentType != ProtobufType {
		t.Errorf("content type = %q, want %q", channel.published.ContentType, ProtobufType)
	}
	if channel.published.MessageId != event.GetMetadata().GetEventId() {
		t.Errorf("message ID = %q, want event ID %q",
			channel.published.MessageId, event.GetMetadata().GetEventId())
	}
	if channel.published.CorrelationId != "saga-123" {
		t.Errorf("correlation ID = %q, want saga-123", channel.published.CorrelationId)
	}

	decoded := &eventsv1.HoldRequested{}
	if err := proto.Unmarshal(channel.published.Body, decoded); err != nil {
		t.Fatalf("published body is not valid protobuf: %v", err)
	}
	if !proto.Equal(event, decoded) {
		t.Errorf("decoded event = %v, want %v", decoded, event)
	}
}

func TestPublishReturnsBrokerError(t *testing.T) {
	wantErr := errors.New("broker unavailable")
	channel := &fakePublisherChannel{publishErr: wantErr}
	publisher, err := NewPublisher(channel)
	if err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}

	err = publisher.Publish(
		context.Background(),
		"hold.requested",
		&eventsv1.HoldRequested{Metadata: &eventsv1.EventMetadata{}},
	)
	if !errors.Is(err, wantErr) {
		t.Errorf("Publish() error = %v, want %v", err, wantErr)
	}
}
