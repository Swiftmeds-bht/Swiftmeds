// Package rabbitmq is the inbound event adapter: it listens for hold.confirmed
// and hands each hold's deadline to the app layer.
package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/app"
)

// HoldConfirmedConsumer stores the deadline of every confirmed hold.
type HoldConfirmedConsumer struct {
	consumer *messaging.Consumer
}

type holdConfirmedChannel interface {
	ExchangeDeclare(
		name, kind string,
		durable, autoDelete, internal, noWait bool,
		args amqp.Table,
	) error
	QueueDeclare(
		name string,
		durable, autoDelete, exclusive, noWait bool,
		args amqp.Table,
	) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	Qos(prefetchCount, prefetchSize int, global bool) error
	ConsumeWithContext(
		ctx context.Context,
		queue, consumer string,
		autoAck, exclusive, noLocal, noWait bool,
		args amqp.Table,
	) (<-chan amqp.Delivery, error)
}

// NewHoldConfirmedConsumer declares the queue expiry.hold-confirmed, bound to
// the routing key hold.confirmed. The deduper makes a redelivered event a
// no-op; a handler error is retried and then dead-lettered.
func NewHoldConfirmedConsumer(
	channel holdConfirmedChannel,
	deduper messaging.Deduper,
	service *app.Service,
) (*HoldConfirmedConsumer, error) {
	if service == nil {
		return nil, errors.New("expiry application service is nil")
	}

	consumer, err := messaging.NewConsumer(
		channel,
		messaging.ConsumerConfig{
			Queue:       "expiry.hold-confirmed",
			RoutingKeys: []string{"hold.confirmed"},
			Prefetch:    10,
			MaxAttempts: 3,
			RetryDelay:  time.Second,
		},
		deduper,
		func(ctx context.Context, delivery amqp.Delivery) error {
			return handleHoldConfirmed(ctx, delivery, service)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create HoldConfirmed consumer: %w", err)
	}
	return &HoldConfirmedConsumer{consumer: consumer}, nil
}

func handleHoldConfirmed(ctx context.Context, delivery amqp.Delivery, service *app.Service) error {
	if delivery.ContentType != messaging.ProtobufType {
		return fmt.Errorf("unexpected content type %q", delivery.ContentType)
	}

	event := &eventsv1.HoldConfirmed{}
	if err := proto.Unmarshal(delivery.Body, event); err != nil {
		return fmt.Errorf("decode HoldConfirmed event: %w", err)
	}
	// A missing timestamp would read as 1970 and expire the hold at once.
	if event.GetExpiresAt() == nil {
		return errors.New("HoldConfirmed event has no expires_at")
	}

	if err := service.RecordDeadline(ctx, event.GetHoldId(), event.GetExpiresAt().AsTime()); err != nil {
		return fmt.Errorf("handle HoldConfirmed event: %w", err)
	}
	return nil
}

// Start consumes events until ctx ends.
func (c *HoldConfirmedConsumer) Start(ctx context.Context) error {
	if c == nil || c.consumer == nil {
		return errors.New("HoldConfirmed consumer is nil")
	}
	return c.consumer.Start(ctx)
}
