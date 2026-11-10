// Package rabbitmq holds Holds' RabbitMQ adapters: the publisher for
// hold.requested and hold.confirmed, and the consumer for stock.reserved and
// stock.unavailable (the reservation saga).
package rabbitmq

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

// Routing keys of the reservation saga.
const (
	RoutingKeyHoldRequested    = "hold.requested"
	RoutingKeyHoldConfirmed    = "hold.confirmed"
	RoutingKeyStockReserved    = "stock.reserved"
	RoutingKeyStockUnavailable = "stock.unavailable"
)

const producerName = "holds"

var _ ports.Events = (*Publisher)(nil)

// eventPublisher is the part of messaging.Publisher that this adapter uses.
type eventPublisher interface {
	Publish(ctx context.Context, routingKey string, event proto.Message) error
}

// Publisher sends Holds' saga events.
type Publisher struct {
	publisher eventPublisher
}

func NewPublisher(publisher eventPublisher) (*Publisher, error) {
	if publisher == nil {
		return nil, errors.New("event publisher is nil")
	}
	return &Publisher{publisher: publisher}, nil
}

// HoldRequested asks Inventory to reserve stock for the hold.
func (p *Publisher) HoldRequested(ctx context.Context, h domain.Hold) error {
	return p.publisher.Publish(ctx, RoutingKeyHoldRequested, &eventsv1.HoldRequested{
		Metadata:   metadata(RoutingKeyHoldRequested, h.ID, h.CreatedAt),
		HoldId:     h.ID,
		UserId:     h.UserID,
		PharmacyId: h.PharmacyID,
		ProductId:  h.ProductID,
		Quantity:   int32(h.Quantity),
	})
}

// HoldConfirmed tells everyone interested that the hold is ACTIVE.
func (p *Publisher) HoldConfirmed(ctx context.Context, h domain.Hold) error {
	if h.Status != domain.Active {
		return fmt.Errorf("hold %s is %s, only an ACTIVE hold can be confirmed", h.ID, h.Status)
	}
	return p.publisher.Publish(ctx, RoutingKeyHoldConfirmed, &eventsv1.HoldConfirmed{
		Metadata:   metadata(RoutingKeyHoldConfirmed, h.ID, h.StatusChangedAt),
		HoldId:     h.ID,
		UserId:     h.UserID,
		PharmacyId: h.PharmacyID,
		ProductId:  h.ProductID,
		Quantity:   int32(h.Quantity),
		PickupCode: h.PickupCode,
		ExpiresAt:  timestamppb.New(h.ExpiresAt),
	})
}

// metadata fills the fields every event carries. The time comes from the hold,
// not from the clock, so sending the same event again gives identical bytes.
func metadata(kind, holdID string, occurredAt time.Time) *eventsv1.EventMetadata {
	return &eventsv1.EventMetadata{
		EventId:       EventID(kind, holdID),
		OccurredAt:    timestamppb.New(occurredAt),
		CorrelationId: holdID,
		Producer:      producerName,
	}
}

// EventID gives the same UUID-shaped ID for the same kind of event about the
// same hold. That is deliberate: if hold.confirmed has to be sent a second time
// (the first publish failed, or the stock.reserved event was delivered twice),
// listeners see the same event ID and ignore the copy.
func EventID(kind, holdID string) string {
	sum := sha256.Sum256([]byte(kind + "/" + holdID))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x50 // version 5 (name based)
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
