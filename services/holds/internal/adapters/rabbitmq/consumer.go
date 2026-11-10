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
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/app"
)

// StockQueue is the queue Holds reads Inventory's answers from.
const StockQueue = "holds.stock-events"

// consumerChannel is the part of an AMQP channel that messaging.NewConsumer uses.
type consumerChannel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	Qos(prefetchCount, prefetchSize int, global bool) error
	ConsumeWithContext(
		ctx context.Context,
		queue, consumer string,
		autoAck, exclusive, noLocal, noWait bool,
		args amqp.Table,
	) (<-chan amqp.Delivery, error)
}

// stockHandler is what the consumer needs from the app layer (*app.Service).
type stockHandler interface {
	HandleStockReserved(ctx context.Context, holdID string) (app.Outcome, error)
	HandleStockUnavailable(ctx context.Context, holdID string) (app.Outcome, error)
}

// StockConsumer reads stock.reserved and stock.unavailable. The shared
// messaging consumer drops an event ID it has already processed, retries a
// failing event, and dead-letters it after the last attempt.
type StockConsumer struct {
	consumer *messaging.Consumer
}

func NewStockConsumer(
	channel consumerChannel,
	deduper messaging.Deduper,
	handler stockHandler,
	log *slog.Logger,
) (*StockConsumer, error) {
	if handler == nil {
		return nil, errors.New("saga handler is nil")
	}
	if log == nil {
		log = slog.Default()
	}

	consumer, err := messaging.NewConsumer(
		channel,
		messaging.ConsumerConfig{
			Queue:       StockQueue,
			RoutingKeys: []string{RoutingKeyStockReserved, RoutingKeyStockUnavailable},
			Prefetch:    10,
			MaxAttempts: 3,
			RetryDelay:  time.Second,
		},
		deduper,
		func(ctx context.Context, delivery amqp.Delivery) error {
			return handleStockEvent(ctx, delivery, handler, log)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create stock event consumer: %w", err)
	}
	return &StockConsumer{consumer: consumer}, nil
}

// Start reads events until ctx ends.
func (c *StockConsumer) Start(ctx context.Context) error {
	if c == nil || c.consumer == nil {
		return errors.New("stock event consumer is nil")
	}
	return c.consumer.Start(ctx)
}

// handleStockEvent decodes one delivery and passes it to the saga. An error
// makes the shared consumer retry the event, and finally dead-letter it.
func handleStockEvent(ctx context.Context, delivery amqp.Delivery, handler stockHandler, log *slog.Logger) error {
	if delivery.ContentType != messaging.ProtobufType {
		return fmt.Errorf("unexpected content type %q", delivery.ContentType)
	}

	var (
		holdID  string
		outcome app.Outcome
		err     error
	)
	switch delivery.RoutingKey {
	case RoutingKeyStockReserved:
		event := &eventsv1.StockReserved{}
		if err := proto.Unmarshal(delivery.Body, event); err != nil {
			return fmt.Errorf("decode %s event: %w", delivery.RoutingKey, err)
		}
		holdID = event.GetHoldId()
		outcome, err = handler.HandleStockReserved(ctx, holdID)
	case RoutingKeyStockUnavailable:
		event := &eventsv1.StockUnavailable{}
		if err := proto.Unmarshal(delivery.Body, event); err != nil {
			return fmt.Errorf("decode %s event: %w", delivery.RoutingKey, err)
		}
		holdID = event.GetHoldId()
		outcome, err = handler.HandleStockUnavailable(ctx, holdID)
	default:
		return fmt.Errorf("unexpected routing key %q", delivery.RoutingKey)
	}
	if err != nil {
		return fmt.Errorf("handle %s for hold %q: %w", delivery.RoutingKey, holdID, err)
	}

	log.InfoContext(ctx, "stock event handled",
		slog.String("routing_key", delivery.RoutingKey),
		slog.String("hold_id", holdID),
		slog.String("message_id", delivery.MessageId),
		slog.String("outcome", string(outcome)))
	return nil
}
