package rabbitmq

import (
	"context"
	"errors"
	"fmt"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
)

type eventPublisher interface {
	Publish(ctx context.Context, routingKey string, event proto.Message) error
}

type Publisher struct {
	publisher eventPublisher
}

func NewPublisher(channel *amqp.Channel) (*Publisher, error) {
	if channel == nil {
		return nil, errors.New("RabbitMQ channel is nil")
	}

	publisher, err := messaging.NewPublisher(channel)
	if err != nil {
		return nil, fmt.Errorf("create event publisher: %w", err)
	}
	return &Publisher{publisher: publisher}, nil
}

func (p *Publisher) PublishStockReserved(
	ctx context.Context,
	holdID, pharmacyID, productID string,
	quantity int32,
) error {
	return p.publish(ctx, "stock.reserved", &eventsv1.StockReserved{
		HoldId:     holdID,
		PharmacyId: pharmacyID,
		ProductId:  productID,
		Quantity:   quantity,
	})
}

func (p *Publisher) PublishStockUnavailable(
	ctx context.Context,
	holdID, pharmacyID, productID string,
	requestedQuantity, availableQuantity int32,
) error {
	return p.publish(ctx, "stock.unavailable", &eventsv1.StockUnavailable{
		HoldId:            holdID,
		PharmacyId:        pharmacyID,
		ProductId:         productID,
		RequestedQuantity: requestedQuantity,
		AvailableQuantity: availableQuantity,
	})
}

func (p *Publisher) PublishStockIncreased(
	ctx context.Context,
	pharmacyID, productID string,
	previousAvailable, newAvailable int32,
) error {
	return p.publish(ctx, "stock.increased", &eventsv1.StockIncreased{
		PharmacyId:        pharmacyID,
		ProductId:         productID,
		PreviousAvailable: previousAvailable,
		NewAvailable:      newAvailable,
	})
}

func (p *Publisher) publish(
	ctx context.Context,
	routingKey string,
	event proto.Message,
) error {
	if p == nil || p.publisher == nil {
		return errors.New("Inventory RabbitMQ publisher is nil")
	}
	if err := p.publisher.Publish(ctx, routingKey, event); err != nil {
		return fmt.Errorf("publish %s: %w", routingKey, err)
	}
	return nil
}

var _ ports.EventPublisher = (*Publisher)(nil)
