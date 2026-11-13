package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/domain"
)

type sent struct {
	key   string
	event proto.Message
}

type fakePublisher struct {
	sent []sent
	err  error
}

func (f *fakePublisher) Publish(_ context.Context, key string, event proto.Message) error {
	f.sent = append(f.sent, sent{key: key, event: event})
	return f.err
}

func TestSearchRecordedPublishesMatchedProduct(t *testing.T) {
	fake := &fakePublisher{}
	publisher, err := NewPublisher(fake)
	if err != nil {
		t.Fatal(err)
	}
	publisher.now = func() time.Time { return time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC) }

	err = publisher.SearchRecorded(context.Background(), "search-1", " Paro ", domain.ProductResult{
		Product: domain.Product{ID: "product-1", BrandName: "Napa", GenericName: "Paracetamol", Strength: "500mg"},
		Offers:  []domain.Offer{{PharmacyID: "pharmacy-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.sent) != 1 || fake.sent[0].key != RoutingKeySearchRecorded {
		t.Fatalf("published = %v, want one search.recorded event", fake.sent)
	}
	got, ok := fake.sent[0].event.(*eventsv1.SearchRecorded)
	if !ok {
		t.Fatalf("event = %T, want *SearchRecorded", fake.sent[0].event)
	}
	if got.GetSearchId() != "search-1" || got.GetProductId() != "product-1" ||
		got.GetProductName() != "Napa 500mg (Paracetamol)" || got.GetDzongkhag() != "Paro" || !got.GetHasStock() {
		t.Fatalf("event = %v", got)
	}
	if got.GetMetadata().GetEventId() != EventID("search-1", "product-1") ||
		got.GetMetadata().GetCorrelationId() != "search-1" || got.GetMetadata().GetProducer() != "search" {
		t.Fatalf("metadata = %v", got.GetMetadata())
	}
}

func TestSearchRecordedWithoutOffersHasNoStock(t *testing.T) {
	fake := &fakePublisher{}
	publisher, _ := NewPublisher(fake)
	if err := publisher.SearchRecorded(context.Background(), "search-1", "", domain.ProductResult{
		Product: domain.Product{ID: "product-1", BrandName: "Napa", Strength: "500mg"},
	}); err != nil {
		t.Fatal(err)
	}
	if fake.sent[0].event.(*eventsv1.SearchRecorded).GetHasStock() {
		t.Error("has_stock = true, want false")
	}
}

func TestSearchRecordedReturnsPublishErrors(t *testing.T) {
	wantErr := errors.New("broker unavailable")
	publisher, _ := NewPublisher(&fakePublisher{err: wantErr})
	if err := publisher.SearchRecorded(context.Background(), "search-1", "", domain.ProductResult{
		Product: domain.Product{ID: "product-1"},
	}); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}
