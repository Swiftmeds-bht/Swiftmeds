package rabbitmq

import (
	"context"
	"crypto/sha256"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/ports"
)

const RoutingKeyAlert = "watchlist.alert"

var _ ports.AlertEvents = (*Publisher)(nil)

type eventPublisher interface {
	Publish(context.Context, string, proto.Message) error
}

type Publisher struct {
	publisher eventPublisher
}

func NewPublisher(publisher eventPublisher) *Publisher {
	return &Publisher{publisher: publisher}
}

func (p *Publisher) AlertCreated(ctx context.Context, a domain.Alert) error {
	return p.publisher.Publish(ctx, RoutingKeyAlert, &eventsv1.WatchlistAlert{
		Metadata: &eventsv1.EventMetadata{
			EventId:       eventID(a.ID),
			OccurredAt:    timestamppb.New(a.OccurredAt),
			CorrelationId: a.ID,
			Producer:      "watchlist",
		},
		AlertId: a.ID, SubscriptionId: a.SubscriptionID, UserId: a.UserID,
		ProductId: a.ProductID, PharmacyId: a.PharmacyID, QuantityAvailable: a.QuantityAvailable,
	})
}

func eventID(alertID string) string {
	sum := sha256.Sum256([]byte("watchlist.alert/" + alertID))
	b := sum[:16]
	b[6] = b[6]&0x0f | 0x50
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
