// Package rabbitmq consumes inventory confirmations for Freshness.
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
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
)

const (
	Queue      = "freshness.stock-increased"
	RoutingKey = "stock.increased"
)

type ConfirmationHandler interface {
	RecordConfirmation(context.Context, string, string, time.Time) (domain.Rating, error)
}

type Channel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	Qos(prefetchCount, prefetchSize int, global bool) error
	ConsumeWithContext(ctx context.Context, queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
}

type StockIncreasedConsumer struct {
	consumer *messaging.Consumer
}

func NewStockIncreasedConsumer(ch Channel, deduper messaging.Deduper, handler ConfirmationHandler, now func() time.Time) (*StockIncreasedConsumer, error) {
	if handler == nil {
		return nil, errors.New("confirmation handler is nil")
	}
	if now == nil {
		now = time.Now
	}
	consumer, err := messaging.NewConsumer(ch, messaging.ConsumerConfig{
		Queue:       Queue,
		RoutingKeys: []string{RoutingKey},
		Prefetch:    10,
		MaxAttempts: 3,
		RetryDelay:  time.Second,
	}, deduper, func(ctx context.Context, delivery amqp.Delivery) error {
		return handle(ctx, delivery, handler, now)
	})
	if err != nil {
		return nil, fmt.Errorf("create stock.increased consumer: %w", err)
	}
	return &StockIncreasedConsumer{consumer: consumer}, nil
}

func (c *StockIncreasedConsumer) Start(ctx context.Context) error {
	if c == nil || c.consumer == nil {
		return errors.New("stock.increased consumer is nil")
	}
	return c.consumer.Start(ctx)
}

func handle(ctx context.Context, delivery amqp.Delivery, handler ConfirmationHandler, now func() time.Time) error {
	if delivery.ContentType != messaging.ProtobufType {
		return fmt.Errorf("unexpected content type %q", delivery.ContentType)
	}
	event := &eventsv1.StockIncreased{}
	if err := proto.Unmarshal(delivery.Body, event); err != nil {
		return fmt.Errorf("decode StockIncreased: %w", err)
	}
	confirmedAt := now()
	if occurredAt := event.GetMetadata().GetOccurredAt(); occurredAt != nil {
		confirmedAt = occurredAt.AsTime()
	}
	if _, err := handler.RecordConfirmation(ctx, event.GetPharmacyId(), event.GetProductId(), confirmedAt); err != nil {
		return fmt.Errorf("record stock confirmation: %w", err)
	}
	return nil
}
