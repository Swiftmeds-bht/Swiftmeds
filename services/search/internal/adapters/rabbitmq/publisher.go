// Package rabbitmq contains Search's outbound event publisher.
package rabbitmq

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/ports"
)

const (
	RoutingKeySearchRecorded = "search.recorded"
	producerName             = "search"
)

var _ ports.SearchEvents = (*Publisher)(nil)

type Publisher struct {
	publisher ports.EventPublisher
	now       func() time.Time
}

func NewPublisher(publisher ports.EventPublisher) (*Publisher, error) {
	if publisher == nil {
		return nil, errors.New("event publisher is nil")
	}
	return &Publisher{publisher: publisher, now: time.Now}, nil
}

func (p *Publisher) SearchRecorded(ctx context.Context, searchID, dzongkhag string, result domain.ProductResult) error {
	if searchID == "" {
		return errors.New("search ID is empty")
	}
	event := &eventsv1.SearchRecorded{
		Metadata: &eventsv1.EventMetadata{
			EventId:       EventID(searchID, result.ID),
			OccurredAt:    timestamppb.New(p.now()),
			CorrelationId: searchID,
			Producer:      producerName,
		},
		SearchId:    searchID,
		ProductId:   result.ID,
		ProductName: result.DisplayName(),
		Dzongkhag:   strings.TrimSpace(dzongkhag),
		HasStock:    len(result.Offers) > 0,
	}
	return p.publisher.Publish(ctx, RoutingKeySearchRecorded, event)
}

func EventID(searchID, productID string) string {
	sum := sha256.Sum256([]byte(searchID + "/" + productID))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
