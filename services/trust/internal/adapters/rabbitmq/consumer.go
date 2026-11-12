// Package rabbitmq is the inbound event adapter: it turns feedback.submitted
// events into app calls. Retries, dead-lettering and deduplication on the
// event ID come from pkg/messaging.
package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
)

const (
	Queue      = "trust.feedback-submitted"
	RoutingKey = "feedback.submitted"
)

// FeedbackHandler is the use case the consumer calls (app.Service).
type FeedbackHandler interface {
	HandleFeedbackSubmitted(ctx context.Context, f domain.Feedback) (bool, error)
}

// Channel is the part of *amqp.Channel the consumer uses.
type Channel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	Qos(prefetchCount, prefetchSize int, global bool) error
	ConsumeWithContext(ctx context.Context, queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
}

// FeedbackSubmittedConsumer feeds feedback.submitted events to the app.
type FeedbackSubmittedConsumer struct {
	consumer *messaging.Consumer
}

// NewFeedbackSubmittedConsumer declares the queue (and its dead-letter queue) and binds it.
func NewFeedbackSubmittedConsumer(ch Channel, deduper messaging.Deduper, handler FeedbackHandler, now func() time.Time) (*FeedbackSubmittedConsumer, error) {
	if handler == nil {
		return nil, errors.New("feedback handler is nil")
	}
	c, err := messaging.NewConsumer(ch, messaging.ConsumerConfig{
		Queue:       Queue,
		RoutingKeys: []string{RoutingKey},
		Prefetch:    10,
		MaxAttempts: 3,
		RetryDelay:  time.Second,
	}, deduper, func(ctx context.Context, d amqp.Delivery) error {
		return handle(ctx, d, handler, now)
	})
	if err != nil {
		return nil, fmt.Errorf("create feedback.submitted consumer: %w", err)
	}
	return &FeedbackSubmittedConsumer{consumer: c}, nil
}

// Start consumes until ctx is cancelled.
func (c *FeedbackSubmittedConsumer) Start(ctx context.Context) error {
	return c.consumer.Start(ctx)
}

func handle(ctx context.Context, d amqp.Delivery, handler FeedbackHandler, now func() time.Time) error {
	if d.ContentType != messaging.ProtobufType {
		return fmt.Errorf("unexpected content type %q", d.ContentType)
	}
	ev := &eventsv1.FeedbackSubmitted{}
	if err := proto.Unmarshal(d.Body, ev); err != nil {
		return fmt.Errorf("decode FeedbackSubmitted: %w", err)
	}
	occurred := now()
	if ts := ev.GetMetadata().GetOccurredAt(); ts != nil {
		occurred = ts.AsTime()
	}
	applied, err := handler.HandleFeedbackSubmitted(ctx, domain.Feedback{
		ID:         ev.GetFeedbackId(),
		PharmacyID: ev.GetPharmacyId(),
		WasInStock: ev.GetWasInStock(),
		OccurredAt: occurred,
	})
	if err != nil {
		return fmt.Errorf("handle FeedbackSubmitted: %w", err)
	}
	if applied {
		slog.InfoContext(ctx, "trust updated from feedback",
			"pharmacy_id", ev.GetPharmacyId(), "feedback_id", ev.GetFeedbackId(), "was_in_stock", ev.GetWasInStock())
	}
	return nil
}
