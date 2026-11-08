// Package rabbitmq publishes Feedback's events to the swiftmed.events exchange.
package rabbitmq

import (
	"context"
	"errors"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/ports"
)

const (
	// RoutingKey is the key of the feedback.submitted event (events.proto).
	RoutingKey = "feedback.submitted"

	producerName = "feedback"
)

var _ ports.EventPublisher = (*Publisher)(nil)

// channel is the part of *amqp.Channel the publisher uses. Tests replace it.
type channel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

type Publisher struct {
	publisher *messaging.Publisher
}

// NewPublisher declares the exchange and returns a publisher on the channel.
func NewPublisher(ch channel) (*Publisher, error) {
	if ch == nil {
		return nil, errors.New("RabbitMQ channel is nil")
	}
	publisher, err := messaging.NewPublisher(ch)
	if err != nil {
		return nil, fmt.Errorf("create event publisher: %w", err)
	}
	return &Publisher{publisher: publisher}, nil
}

// PublishFeedbackSubmitted sends feedback.submitted. messaging.Publisher fills
// in metadata.event_id with a new UUID. The event deliberately carries no
// user_id (events.proto, risk R8), and the hold ID is its correlation ID.
func (p *Publisher) PublishFeedbackSubmitted(ctx context.Context, f domain.Feedback) error {
	if p == nil || p.publisher == nil {
		return errors.New("feedback RabbitMQ publisher is nil")
	}
	event := &eventsv1.FeedbackSubmitted{
		Metadata: &eventsv1.EventMetadata{
			OccurredAt:    timestamppb.New(f.CreatedAt),
			CorrelationId: f.HoldID,
			Producer:      producerName,
		},
		FeedbackId: f.ID,
		HoldId:     f.HoldID,
		PharmacyId: f.PharmacyID,
		ProductId:  f.ProductID,
		WasInStock: f.WasInStock,
		Rating:     int32(f.Rating),
	}
	if err := p.publisher.Publish(ctx, RoutingKey, event); err != nil {
		return fmt.Errorf("publish %s: %w", RoutingKey, err)
	}
	return nil
}
