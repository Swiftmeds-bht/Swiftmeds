package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
)

type HoldRequestedConsumer struct {
	consumer *messaging.Consumer
}

func NewHoldRequestedConsumer(
	channel *amqp.Channel,
	deduper messaging.Deduper,
	service *app.Service,
) (*HoldRequestedConsumer, error) {
	if service == nil {
		return nil, errors.New("inventory application service is nil")
	}

	consumer, err := messaging.NewConsumer(
		channel,
		messaging.ConsumerConfig{
			Queue:       "inventory.hold-requested",
			RoutingKeys: []string{"hold.requested"},
			Prefetch:    10,
			MaxAttempts: 3,
			RetryDelay:  time.Second,
		},
		deduper,
		func(ctx context.Context, delivery amqp.Delivery) error {
			return handleHoldRequested(ctx, delivery, service)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create HoldRequested consumer: %w", err)
	}
	return &HoldRequestedConsumer{consumer: consumer}, nil
}

func handleHoldRequested(ctx context.Context, delivery amqp.Delivery, service *app.Service) error {
	if service == nil {
		return errors.New("inventory application service is nil")
	}
	if delivery.ContentType != messaging.ProtobufType {
		return fmt.Errorf("unexpected content type %q", delivery.ContentType)
	}
	event := &eventsv1.HoldRequested{}
	if err := proto.Unmarshal(delivery.Body, event); err != nil {
		return fmt.Errorf("decode HoldRequested event: %w", err)
	}
	_, _, err := service.Reserve(
		ctx,
		event.GetHoldId(),
		ports.StockKey{PharmacyID: event.GetPharmacyId(), ProductID: event.GetProductId()},
		event.GetQuantity(),
	)
	if err != nil {
		return fmt.Errorf("handle HoldRequested event: %w", err)
	}
	return nil
}

func (c *HoldRequestedConsumer) Start(ctx context.Context) error {
	if c == nil || c.consumer == nil {
		return errors.New("HoldRequested consumer is nil")
	}
	return c.consumer.Start(ctx)
}
