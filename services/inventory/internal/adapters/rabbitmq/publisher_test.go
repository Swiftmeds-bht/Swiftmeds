package rabbitmq

import (
	"context"
	"errors"
	"testing"

	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
)

var errPublishTest = errors.New("publish test failure")

type publisherCapture struct {
	routingKey string
	event      proto.Message
	err        error
}

func (f *publisherCapture) Publish(_ context.Context, routingKey string, event proto.Message) error {
	f.routingKey = routingKey
	f.event = event
	return f.err
}

func TestNewPublisherRejectsNilChannel(t *testing.T) {
	if _, err := NewPublisher((*amqp.Channel)(nil)); err == nil {
		t.Fatal("NewPublisher(nil) returned no error")
	}
}

func TestPublishStockEvents(t *testing.T) {
	capture := &publisherCapture{}
	publisher := &Publisher{publisher: capture}

	if err := publisher.PublishStockReserved(context.Background(), "hold-1", "pharmacy-1", "product-1", 3); err != nil {
		t.Fatalf("PublishStockReserved: %v", err)
	}
	reserved, ok := capture.event.(*eventsv1.StockReserved)
	if !ok || capture.routingKey != "stock.reserved" || reserved.GetHoldId() != "hold-1" || reserved.GetPharmacyId() != "pharmacy-1" || reserved.GetProductId() != "product-1" || reserved.GetQuantity() != 3 {
		t.Fatalf("reserved publish = key %q event %T %+v", capture.routingKey, capture.event, capture.event)
	}

	if err := publisher.PublishStockUnavailable(context.Background(), "hold-2", "pharmacy-2", "product-2", 5, 1); err != nil {
		t.Fatalf("PublishStockUnavailable: %v", err)
	}
	unavailable, ok := capture.event.(*eventsv1.StockUnavailable)
	if !ok || capture.routingKey != "stock.unavailable" || unavailable.GetHoldId() != "hold-2" || unavailable.GetRequestedQuantity() != 5 || unavailable.GetAvailableQuantity() != 1 {
		t.Fatalf("unavailable publish = key %q event %T %+v", capture.routingKey, capture.event, capture.event)
	}

	if err := publisher.PublishStockIncreased(context.Background(), "pharmacy-3", "product-3", 2, 7); err != nil {
		t.Fatalf("PublishStockIncreased: %v", err)
	}
	increased, ok := capture.event.(*eventsv1.StockIncreased)
	if !ok || capture.routingKey != "stock.increased" || increased.GetPharmacyId() != "pharmacy-3" || increased.GetProductId() != "product-3" || increased.GetPreviousAvailable() != 2 || increased.GetNewAvailable() != 7 {
		t.Fatalf("increased publish = key %q event %T %+v", capture.routingKey, capture.event, capture.event)
	}
}

func TestPublishPropagatesErrorAndRejectsNilPublisher(t *testing.T) {
	capture := &publisherCapture{err: errPublishTest}
	publisher := &Publisher{publisher: capture}
	if err := publisher.PublishStockReserved(context.Background(), "hold-1", "pharmacy-1", "product-1", 1); !errors.Is(err, errPublishTest) {
		t.Fatalf("PublishStockReserved error = %v, want wrapped publisher error", err)
	}
	if err := (*Publisher)(nil).PublishStockIncreased(context.Background(), "pharmacy-1", "product-1", 0, 1); err == nil {
		t.Fatal("nil publisher returned no error")
	}
	if err := (&Publisher{}).PublishStockUnavailable(context.Background(), "hold-1", "pharmacy-1", "product-1", 1, 0); err == nil {
		t.Fatal("publisher without a messaging publisher returned no error")
	}
}
