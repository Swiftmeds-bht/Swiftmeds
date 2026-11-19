package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
)

type recordingHandler struct {
	pharmacyID string
	productID  string
	confirmed  time.Time
	err        error
}

func (h *recordingHandler) RecordConfirmation(_ context.Context, pharmacyID, productID string, confirmedAt time.Time) (domain.Rating, error) {
	h.pharmacyID, h.productID, h.confirmed = pharmacyID, productID, confirmedAt
	return domain.Rating{}, h.err
}

func TestStockIncreasedEventRecordsConfirmation(t *testing.T) {
	occurredAt := time.Date(2026, 11, 19, 8, 0, 0, 0, time.UTC)
	body, err := proto.Marshal(&eventsv1.StockIncreased{
		Metadata:     &eventsv1.EventMetadata{OccurredAt: timestamppb.New(occurredAt)},
		PharmacyId:   "pharmacy-1",
		ProductId:    "product-1",
		NewAvailable: 4,
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := &recordingHandler{}
	err = handle(context.Background(), amqp.Delivery{
		ContentType: messaging.ProtobufType,
		Body:        body,
	}, handler, func() time.Time { return occurredAt.Add(time.Hour) })
	if err != nil {
		t.Fatal(err)
	}
	if handler.pharmacyID != "pharmacy-1" || handler.productID != "product-1" {
		t.Fatalf("confirmation key = %q/%q, want pharmacy-1/product-1", handler.pharmacyID, handler.productID)
	}
	if !handler.confirmed.Equal(occurredAt) {
		t.Errorf("confirmed_at = %v, want %v", handler.confirmed, occurredAt)
	}
}

func TestStockIncreasedEventUsesClockWithoutOccurredAt(t *testing.T) {
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	handler := &recordingHandler{}
	body, err := proto.Marshal(&eventsv1.StockIncreased{PharmacyId: "pharmacy-1", ProductId: "product-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := handle(context.Background(), amqp.Delivery{ContentType: messaging.ProtobufType, Body: body}, handler, func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	if !handler.confirmed.Equal(now) {
		t.Errorf("confirmed_at = %v, want %v", handler.confirmed, now)
	}
}

func TestStockIncreasedEventRejectsInvalidDelivery(t *testing.T) {
	handler := &recordingHandler{}
	if err := handle(context.Background(), amqp.Delivery{ContentType: "application/json", Body: []byte("{}")}, handler, time.Now); err == nil {
		t.Fatal("accepted a non-protobuf delivery")
	}
	if err := handle(context.Background(), amqp.Delivery{ContentType: messaging.ProtobufType, Body: []byte{0xff}}, handler, time.Now); err == nil {
		t.Fatal("accepted an invalid protobuf payload")
	}
	handler.err = errors.New("database unavailable")
	body, _ := proto.Marshal(&eventsv1.StockIncreased{PharmacyId: "pharmacy-1", ProductId: "product-1"})
	if err := handle(context.Background(), amqp.Delivery{ContentType: messaging.ProtobufType, Body: body}, handler, time.Now); err == nil {
		t.Fatal("swallowed confirmation error")
	}
}
