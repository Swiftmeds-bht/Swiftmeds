package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewStock(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	stock, err := NewStock("pharmacy-1", "product-1", 10, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := stock.QuantityAvailable(); got != 8 {
		t.Fatalf("available = %d, want 8", got)
	}
	if !stock.ConfirmedAt.Equal(now) {
		t.Fatalf("confirmed_at = %v, want %v", stock.ConfirmedAt, now)
	}
}

func TestNewStockRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name                  string
		pharmacyID, productID string
		onHand, reserved      int32
		wantErr               error
	}{
		{"missing pharmacy", "", "product-1", 5, 0, ErrInvalidStockKey},
		{"missing product", "pharmacy-1", "", 5, 0, ErrInvalidStockKey},
		{"negative on hand", "pharmacy-1", "product-1", -1, 0, ErrNegativeStock},
		{"reserved exceeds on hand", "pharmacy-1", "product-1", 2, 3, ErrOnHandBelowReservedQuantity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewStock(tt.pharmacyID, tt.productID, tt.onHand, tt.reserved, time.Time{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetQuantityOnHand(t *testing.T) {
	stock, err := NewStock("pharmacy-1", "product-1", 10, 2, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := stock.SetQuantityOnHand(8)
	if err != nil {
		t.Fatal(err)
	}
	if updated.QuantityOnHand != 8 || updated.QuantityReserved != 2 || updated.QuantityAvailable() != 6 {
		t.Fatalf("unexpected stock after set: %+v", updated)
	}

	if _, err := stock.SetQuantityOnHand(1); !errors.Is(err, ErrOnHandBelowReservedQuantity) {
		t.Fatalf("setting below reserved: error = %v", err)
	}
	if _, err := stock.SetQuantityOnHand(-1); !errors.Is(err, ErrNegativeStock) {
		t.Fatalf("setting negative on hand: error = %v", err)
	}
}

func TestReserve(t *testing.T) {
	stock, err := NewStock("pharmacy-1", "product-1", 10, 3, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	reserved, err := stock.Reserve(4)
	if err != nil {
		t.Fatal(err)
	}
	if reserved.QuantityReserved != 7 || reserved.QuantityAvailable() != 3 {
		t.Fatalf("unexpected stock after reserve: %+v", reserved)
	}

	if _, err := stock.Reserve(0); !errors.Is(err, ErrInvalidReservationQuantity) {
		t.Fatalf("reserving zero: error = %v", err)
	}
	if _, err := stock.Reserve(8); !errors.Is(err, ErrInsufficientAvailableStock) {
		t.Fatalf("reserving too much: error = %v", err)
	}
}

func TestCommitReservation(t *testing.T) {
	stock, err := NewStock("pharmacy-1", "product-1", 10, 5, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	committed, err := stock.CommitReservation(2)
	if err != nil {
		t.Fatal(err)
	}
	if committed.QuantityOnHand != 8 || committed.QuantityReserved != 3 || committed.QuantityAvailable() != 5 {
		t.Fatalf("unexpected stock after commit: %+v", committed)
	}

	if _, err := stock.CommitReservation(0); !errors.Is(err, ErrInvalidReservationQuantity) {
		t.Fatalf("committing zero: error = %v", err)
	}
	if _, err := stock.CommitReservation(6); !errors.Is(err, ErrInsufficientReservedStock) {
		t.Fatalf("committing too much: error = %v", err)
	}
}

func TestReleaseReservation(t *testing.T) {
	stock, err := NewStock("pharmacy-1", "product-1", 10, 5, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	released, err := stock.ReleaseReservation(2)
	if err != nil {
		t.Fatal(err)
	}
	if released.QuantityOnHand != 10 || released.QuantityReserved != 3 || released.QuantityAvailable() != 7 {
		t.Fatalf("unexpected stock after release: %+v", released)
	}

	if _, err := stock.ReleaseReservation(0); !errors.Is(err, ErrInvalidReservationQuantity) {
		t.Fatalf("releasing zero: error = %v", err)
	}
	if _, err := stock.ReleaseReservation(6); !errors.Is(err, ErrInsufficientReservedStock) {
		t.Fatalf("releasing too much: error = %v", err)
	}
}
