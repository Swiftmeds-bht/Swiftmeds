package rabbitmq

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	notificationv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/notification/v1"
)

func TestParseDeliveryMapsTemplatesAndUser(t *testing.T) {
	tests := []struct {
		name, key, userID string
		event             proto.Message
		wantTemplate      notificationv1.Template
	}{
		{
			name: "hold confirmed", key: "hold.confirmed", userID: "u-1", wantTemplate: notificationv1.Template_TEMPLATE_HOLD_CONFIRMED,
			event: &eventsv1.HoldConfirmed{Metadata: &eventsv1.EventMetadata{EventId: "evt-1"}, UserId: "u-1", HoldId: "h-1", PickupCode: "9911"},
		},
		{
			name: "hold expired", key: "hold.expired", userID: "u-2", wantTemplate: notificationv1.Template_TEMPLATE_HOLD_EXPIRED,
			event: &eventsv1.HoldExpired{Metadata: &eventsv1.EventMetadata{EventId: "evt-1"}, UserId: "u-2", HoldId: "h-2"},
		},
		{
			name: "hold rejected", key: "hold.rejected", userID: "u-3", wantTemplate: notificationv1.Template_TEMPLATE_HOLD_REJECTED,
			event: &eventsv1.HoldRejected{Metadata: &eventsv1.EventMetadata{EventId: "evt-1"}, UserId: "u-3", HoldId: "h-3", Reason: "stock unavailable"},
		},
		{
			name: "watchlist alert", key: "watchlist.alert", userID: "u-4", wantTemplate: notificationv1.Template_TEMPLATE_BACK_IN_STOCK,
			event: &eventsv1.WatchlistAlert{Metadata: &eventsv1.EventMetadata{EventId: "evt-1"}, UserId: "u-4", ProductId: "p-1", PharmacyId: "ph-1", QuantityAvailable: 2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, err := proto.Marshal(tc.event)
			if err != nil {
				t.Fatalf("marshal event: %v", err)
			}
			_, user, template, _, err := parseDelivery(amqp.Delivery{
				RoutingKey:  tc.key,
				MessageId:   "evt-1",
				ContentType: messaging.ProtobufType,
				Body:        body,
			})
			if err != nil {
				t.Fatalf("parse delivery: %v", err)
			}
			if user != tc.userID {
				t.Fatalf("user = %q, want %q", user, tc.userID)
			}
			if template != tc.wantTemplate {
				t.Fatalf("template = %v, want %v", template, tc.wantTemplate)
			}
		})
	}
}
