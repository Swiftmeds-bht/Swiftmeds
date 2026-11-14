package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/domain"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
)

const Queue = "analytics.events"

const (
	SearchRecordedKey   = "search.recorded"
	StockUnavailableKey = "stock.unavailable"
)

type channel interface {
	ExchangeDeclare(string, string, bool, bool, bool, bool, amqp.Table) error
	QueueDeclare(string, bool, bool, bool, bool, amqp.Table) (amqp.Queue, error)
	QueueBind(string, string, string, bool, amqp.Table) error
	Qos(int, int, bool) error
	ConsumeWithContext(context.Context, string, string, bool, bool, bool, bool, amqp.Table) (<-chan amqp.Delivery, error)
}

type Consumer struct{ inner *messaging.Consumer }

func NewConsumer(ch channel, deduper messaging.Deduper, svc *app.Service, log *slog.Logger) (*Consumer, error) {
	if svc == nil {
		return nil, errors.New("analytics service is nil")
	}
	if log == nil {
		log = slog.Default()
	}
	inner, err := messaging.NewConsumer(ch, messaging.ConsumerConfig{
		Queue: Queue, RoutingKeys: []string{SearchRecordedKey, StockUnavailableKey},
		Prefetch: 10, MaxAttempts: 3, RetryDelay: time.Second,
	}, deduper, func(ctx context.Context, d amqp.Delivery) error {
		if d.ContentType != messaging.ProtobufType {
			return fmt.Errorf("unexpected content type %q", d.ContentType)
		}
		var err error
		switch d.RoutingKey {
		case SearchRecordedKey:
			ev := &eventsv1.SearchRecorded{}
			if err = proto.Unmarshal(d.Body, ev); err == nil {
				ts := ev.GetMetadata().GetOccurredAt()
				when := time.Now()
				if ts != nil {
					when = ts.AsTime()
				}
				err = svc.RecordSearch(ctx, domain.SearchRecorded{
					EventID: ev.GetMetadata().GetEventId(), SearchID: ev.GetSearchId(),
					ProductID: ev.GetProductId(), ProductName: ev.GetProductName(),
					Dzongkhag: ev.GetDzongkhag(), OccurredAt: when, HasStock: ev.GetHasStock(),
				})
			}
		case StockUnavailableKey:
			ev := &eventsv1.StockUnavailable{}
			if err = proto.Unmarshal(d.Body, ev); err == nil {
				ts := ev.GetMetadata().GetOccurredAt()
				when := time.Now()
				if ts != nil {
					when = ts.AsTime()
				}
				err = svc.RecordStockUnavailable(ctx, domain.StockUnavailable{
					EventID: ev.GetMetadata().GetEventId(), PharmacyID: ev.GetPharmacyId(),
					ProductID: ev.GetProductId(), OccurredAt: when,
				})
			}
		default:
			return fmt.Errorf("unexpected routing key %q", d.RoutingKey)
		}
		if err != nil {
			return fmt.Errorf("handle %s: %w", d.RoutingKey, err)
		}
		log.InfoContext(ctx, "analytics event recorded", "routing_key", d.RoutingKey)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create analytics consumer: %w", err)
	}
	return &Consumer{inner: inner}, nil
}

func (c *Consumer) Start(ctx context.Context) error { return c.inner.Start(ctx) }
