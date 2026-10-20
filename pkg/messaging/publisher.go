package messaging

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	ExchangeName = "swiftmed.events"
	ProtobufType = "application/protobuf"
)

type publisherChannel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

// Publisher sends protobuf events to RabbitMQ.
type Publisher struct {
	channel publisherChannel
}

// NewPublisher declares the durable topic exchange and returns a publisher.
func NewPublisher(channel publisherChannel) (*Publisher, error) {
	if channel == nil {
		return nil, errors.New("RabbitMQ channel is nil")
	}

	if err := channel.ExchangeDeclare(
		ExchangeName, "topic", true, false, false, false, nil,
	); err != nil {
		return nil, fmt.Errorf("declare exchange %q: %w", ExchangeName, err)
	}

	return &Publisher{channel: channel}, nil
}

// Publish adds a missing event ID, serializes the protobuf, and publishes it persistently.
func (p *Publisher) Publish(ctx context.Context, routingKey string, event proto.Message) error {
	if p == nil || p.channel == nil {
		return errors.New("publisher is nil")
	}
	if event == nil {
		return errors.New("event is nil")
	}
	value := reflect.ValueOf(event)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return errors.New("event is nil")
	}
	if strings.TrimSpace(routingKey) == "" {
		return errors.New("routing key is empty")
	}

	eventID, correlationID, err := ensureEventMetadata(event)
	if err != nil {
		return err
	}

	body, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	err = p.channel.PublishWithContext(
		ctx,
		ExchangeName,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:   ProtobufType,
			DeliveryMode:  amqp.Persistent,
			MessageId:     eventID,
			CorrelationId: correlationID,
			Body:          body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish event %q: %w", eventID, err)
	}
	return nil
}

func ensureEventMetadata(event proto.Message) (eventID, correlationID string, err error) {
	message := event.ProtoReflect()
	metadataField := message.Descriptor().Fields().ByName("metadata")
	if metadataField == nil || metadataField.Kind() != protoreflect.MessageKind {
		return "", "", errors.New("event has no protobuf metadata message")
	}

	metadata := message.Mutable(metadataField).Message()
	eventIDField := metadata.Descriptor().Fields().ByName("event_id")
	if eventIDField == nil || eventIDField.Kind() != protoreflect.StringKind {
		return "", "", errors.New("event metadata has no string event_id field")
	}

	eventID = metadata.Get(eventIDField).String()
	if eventID == "" {
		eventID, err = newEventID()
		if err != nil {
			return "", "", err
		}
		metadata.Set(eventIDField, protoreflect.ValueOfString(eventID))
	}

	correlationField := metadata.Descriptor().Fields().ByName("correlation_id")
	if correlationField != nil && correlationField.Kind() == protoreflect.StringKind {
		correlationID = metadata.Get(correlationField).String()
	}
	return eventID, correlationID, nil
}

func newEventID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate event ID: %w", err)
	}

	// Set the UUID version 4 and variant bits.
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		id[0:4], id[4:6], id[6:8], id[8:10], id[10:16],
	), nil
}
