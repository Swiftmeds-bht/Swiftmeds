package app

import (
	"context"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
)

// A bulk update publishes stock.increased only for rows whose available quantity
// went up, and keeps checking the rows after one that did not (SCRUM-113).
func TestBulkSetSkipsUnchangedRowsAndKeepsGoing(t *testing.T) {
	publisher := &publisherFake{}
	repo := &repositoryFake{
		bulkSetFn: func(context.Context, string, []ports.StockUpdate, string) ([]ports.StockChange, []ports.StockUpdateError, error) {
			return []ports.StockChange{
				// Unchanged: 5 available before and after. Must not publish.
				{Stock: testStock(t, "pharmacy-1", "product-1", 6, 1), PreviousAvailable: 5},
				// Went down. Must not publish.
				{Stock: testStock(t, "pharmacy-1", "product-2", 1, 0), PreviousAvailable: 2},
				// Went up by exactly one, from nothing. Must publish.
				{Stock: testStock(t, "pharmacy-1", "product-3", 1, 0), PreviousAvailable: 0},
				// Went up. Must publish.
				{Stock: testStock(t, "pharmacy-1", "product-4", 9, 2), PreviousAvailable: 3},
			}, nil, nil
		},
	}
	service := newTestService(t, repo, publisher)

	changes, _, err := service.BulkSetStock(context.Background(), "pharmacy-1", nil, "user-1")
	if err != nil {
		t.Fatalf("BulkSetStock: %v", err)
	}
	if len(changes) != 4 {
		t.Fatalf("changes = %d, want 4 (all rows returned)", len(changes))
	}
	want := []increasedEvent{
		{pharmacyID: "pharmacy-1", productID: "product-3", previousAvailable: 0, newAvailable: 1},
		{pharmacyID: "pharmacy-1", productID: "product-4", previousAvailable: 3, newAvailable: 7},
	}
	if len(publisher.increased) != len(want) {
		t.Fatalf("stock.increased events = %+v, want %+v", publisher.increased, want)
	}
	for i := range want {
		if publisher.increased[i] != want[i] {
			t.Fatalf("event %d = %+v, want %+v", i, publisher.increased[i], want[i])
		}
	}
}

func TestSetStockPublishesWhenAvailableGoesFromZeroToOne(t *testing.T) {
	publisher := &publisherFake{}
	repo := &repositoryFake{
		setFn: func(_ context.Context, requested domain.Stock, _ string) (int32, domain.Stock, error) {
			if requested.QuantityReserved != 0 {
				t.Errorf("requested reserved = %d, want 0 (repository keeps the real value)", requested.QuantityReserved)
			}
			return 0, testStock(t, requested.PharmacyID, requested.ProductID, 1, 0), nil
		},
	}
	service := newTestService(t, repo, publisher)

	if _, err := service.SetStock(context.Background(), "pharmacy-1", "product-1", 1, "user-1"); err != nil {
		t.Fatalf("SetStock: %v", err)
	}
	want := increasedEvent{pharmacyID: "pharmacy-1", productID: "product-1", previousAvailable: 0, newAvailable: 1}
	if len(publisher.increased) != 1 || publisher.increased[0] != want {
		t.Fatalf("stock.increased events = %+v, want [%+v]", publisher.increased, want)
	}
}
