package rabbitmq

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
)

type recordedPublication struct {
	key   string
	event proto.Message
}

type recordingEventPublisher struct{ calls []recordedPublication }

func (p *recordingEventPublisher) Publish(_ context.Context, key string, event proto.Message) error {
	p.calls = append(p.calls, recordedPublication{key: key, event: event})
	return nil
}

func TestAlertCreatedPublishesNotificationEventForAlertUser(t *testing.T) {
	publisher := &recordingEventPublisher{}
	adapter := NewPublisher(publisher)
	alert := domain.Alert{
		ID: "alert-1", SubscriptionID: "sub-1", UserID: "patient-1", ProductID: "product-1",
		PharmacyID: "pharmacy-1", QuantityAvailable: 4, OccurredAt: time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC),
	}
	if err := adapter.AlertCreated(context.Background(), alert); err != nil {
		t.Fatal(err)
	}
	if len(publisher.calls) != 1 || publisher.calls[0].key != RoutingKeyAlert {
		t.Fatalf("publications = %+v, want one %s event", publisher.calls, RoutingKeyAlert)
	}
	event, ok := publisher.calls[0].event.(*eventsv1.WatchlistAlert)
	if !ok {
		t.Fatalf("event type = %T, want *WatchlistAlert", publisher.calls[0].event)
	}
	if event.GetUserId() != alert.UserID || event.GetAlertId() != alert.ID || event.GetSubscriptionId() != alert.SubscriptionID ||
		event.GetProductId() != alert.ProductID || event.GetPharmacyId() != alert.PharmacyID || event.GetQuantityAvailable() != alert.QuantityAvailable {
		t.Errorf("event = %+v, want details from alert %+v", event, alert)
	}
	if event.GetMetadata().GetEventId() != eventID(alert.ID) {
		t.Errorf("metadata.event_id = %q, want %q", event.GetMetadata().GetEventId(), eventID(alert.ID))
	}
}
