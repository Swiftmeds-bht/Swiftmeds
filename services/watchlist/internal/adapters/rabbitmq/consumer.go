// Package rabbitmq is the inbound event adapter: it turns stock.increased
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
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
)

const (
	Queue      = "watchlist.stock-increased"
	RoutingKey = "stock.increased"
)

// StockHandler is the use case the consumer calls (app.Service).
type StockHandler interface {
	HandleStockIncreased(ctx context.Context, e domain.StockIncrease) ([]domain.Alert, error)
}

// Channel is the part of *amqp.Channel the consumer uses.
type Channel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	Qos(prefetchCount, prefetchSize int, global bool) error
	ConsumeWithContext(ctx context.Context, queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
}

// StockIncreasedConsumer feeds stock.increased events to the app.
type StockIncreasedConsumer struct {
	consumer *messaging.Consumer
}

// NewStockIncreasedConsumer declares the queue (and its dead-letter queue) and binds it.
func NewStockIncreasedConsumer(ch Channel, deduper messaging.Deduper, handler StockHandler, now func() time.Time) (*StockIncreasedConsumer, error) {
	if handler == nil {
		return nil, errors.New("stock handler is nil")
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
		return nil, fmt.Errorf("create stock.increased consumer: %w", err)
	}
	return &StockIncreasedConsumer{consumer: c}, nil
}

// Start consumes until ctx is cancelled.
func (c *StockIncreasedConsumer) Start(ctx context.Context) error {
	return c.consumer.Start(ctx)
}

func handle(ctx context.Context, d amqp.Delivery, handler StockHandler, now func() time.Time) error {
	if d.ContentType != messaging.ProtobufType {
		return fmt.Errorf("unexpected content type %q", d.ContentType)
	}
	ev := &eventsv1.StockIncreased{}
	if err := proto.Unmarshal(d.Body, ev); err != nil {
		return fmt.Errorf("decode StockIncreased: %w", err)
	}
	occurred := now()
	if ts := ev.GetMetadata().GetOccurredAt(); ts != nil {
		occurred = ts.AsTime()
	}
	alerts, err := handler.HandleStockIncreased(ctx, domain.StockIncrease{
		PharmacyID:   ev.GetPharmacyId(),
		ProductID:    ev.GetProductId(),
		NewAvailable: ev.GetNewAvailable(),
		OccurredAt:   occurred,
	})
	if err != nil {
		return fmt.Errorf("handle StockIncreased: %w", err)
	}
	if len(alerts) > 0 {
		slog.InfoContext(ctx, "back-in-stock alerts recorded",
			"product_id", ev.GetProductId(), "pharmacy_id", ev.GetPharmacyId(), "alerts", len(alerts))
	}
	return nil
}
