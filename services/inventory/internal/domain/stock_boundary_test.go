package domain

import (
	"errors"
	"testing"
	"time"
)

// Boundary tests for the stock rules (SCRUM-113). Each case sits exactly on a
// limit, so turning a < into <= (or > into >=) in stock.go makes one of them fail.

func mustStock(t *testing.T, onHand, reserved int32) Stock {
	t.Helper()
	s, err := NewStock("pharmacy-1", "product-1", onHand, reserved, time.Time{})
	if err != nil {
		t.Fatalf("NewStock(%d, %d): %v", onHand, reserved, err)
	}
	return s
}

func TestNewStockBoundaries(t *testing.T) {
	tests := []struct {
		name             string
		onHand, reserved int32
		wantErr          error
		wantAvailable    int32
	}{
		{"empty shelf is valid", 0, 0, nil, 0},
		{"nothing reserved is valid", 4, 0, nil, 4},
		{"everything reserved is valid", 4, 4, nil, 0},
		{"one more reserved than on hand", 4, 5, ErrOnHandBelowReservedQuantity, 0},
		{"negative reserved", 4, -1, ErrNegativeStock, 0},
		{"negative on hand with nothing reserved", -1, 0, ErrNegativeStock, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewStock("pharmacy-1", "product-1", tt.onHand, tt.reserved, time.Time{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				if s != (Stock{}) {
					t.Fatalf("stock on error = %+v, want zero value", s)
				}
				return
			}
			if s.QuantityOnHand != tt.onHand || s.QuantityReserved != tt.reserved {
				t.Fatalf("stock = %+v, want on hand %d, reserved %d", s, tt.onHand, tt.reserved)
			}
			if got := s.QuantityAvailable(); got != tt.wantAvailable {
				t.Fatalf("available = %d, want %d", got, tt.wantAvailable)
			}
		})
	}
}

func TestNewStockRejectsBlankKeys(t *testing.T) {
	for _, ids := range [][2]string{{"  ", "product-1"}, {"pharmacy-1", "\t"}} {
		if _, err := NewStock(ids[0], ids[1], 1, 0, time.Time{}); !errors.Is(err, ErrInvalidStockKey) {
			t.Fatalf("NewStock(%q, %q) error = %v, want ErrInvalidStockKey", ids[0], ids[1], err)
		}
	}
}

func TestSetQuantityOnHandBoundaries(t *testing.T) {
	t.Run("zero when nothing is reserved", func(t *testing.T) {
		got, err := mustStock(t, 3, 0).SetQuantityOnHand(0)
		if err != nil {
			t.Fatalf("SetQuantityOnHand(0): %v", err)
		}
		if got.QuantityOnHand != 0 || got.QuantityAvailable() != 0 {
			t.Fatalf("stock = %+v, want empty", got)
		}
	})
	t.Run("exactly the reserved quantity", func(t *testing.T) {
		got, err := mustStock(t, 10, 3).SetQuantityOnHand(3)
		if err != nil {
			t.Fatalf("SetQuantityOnHand(3): %v", err)
		}
		if got.QuantityOnHand != 3 || got.QuantityReserved != 3 || got.QuantityAvailable() != 0 {
			t.Fatalf("stock = %+v, want 3 on hand, 3 reserved", got)
		}
	})
	t.Run("one below the reserved quantity", func(t *testing.T) {
		orig := mustStock(t, 10, 3)
		got, err := orig.SetQuantityOnHand(2)
		if !errors.Is(err, ErrOnHandBelowReservedQuantity) {
			t.Fatalf("error = %v, want ErrOnHandBelowReservedQuantity", err)
		}
		if got != orig {
			t.Fatalf("stock changed on error: %+v", got)
		}
	})
}

// The reservation rules guard against overselling: available = 0 and 1 and
// quantity just above available are the cases that matter.
func TestReserveBoundaries(t *testing.T) {
	tests := []struct {
		name             string
		onHand, reserved int32
		quantity         int32
		wantErr          error
		wantReserved     int32
	}{
		{"nothing available", 3, 3, 1, ErrInsufficientAvailableStock, 3},
		{"empty shelf", 0, 0, 1, ErrInsufficientAvailableStock, 0},
		{"last unit", 4, 3, 1, nil, 4},
		{"one more than the last unit", 4, 3, 2, ErrInsufficientAvailableStock, 3},
		{"exactly what is available", 5, 0, 5, nil, 5},
		{"one more than available", 5, 0, 6, ErrInsufficientAvailableStock, 0},
		{"smallest valid quantity", 5, 0, 1, nil, 1},
		{"zero", 5, 0, 0, ErrInvalidReservationQuantity, 0},
		{"negative", 5, 0, -1, ErrInvalidReservationQuantity, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := mustStock(t, tt.onHand, tt.reserved)
			got, err := orig.Reserve(tt.quantity)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Reserve(%d) error = %v, want %v", tt.quantity, err, tt.wantErr)
			}
			if err != nil && got != orig {
				t.Fatalf("stock changed on error: %+v", got)
			}
			if got.QuantityReserved != tt.wantReserved || got.QuantityOnHand != tt.onHand {
				t.Fatalf("stock = %+v, want on hand %d, reserved %d", got, tt.onHand, tt.wantReserved)
			}
			if got.QuantityAvailable() < 0 {
				t.Fatalf("available went negative: %+v", got)
			}
		})
	}
}

func TestReserveLeavesOriginalUnchanged(t *testing.T) {
	orig := mustStock(t, 5, 1)
	if _, err := orig.Reserve(2); err != nil {
		t.Fatal(err)
	}
	if orig.QuantityReserved != 1 {
		t.Fatalf("original reserved = %d, want 1 (Reserve must return a copy)", orig.QuantityReserved)
	}
}

func TestCommitAndReleaseBoundaries(t *testing.T) {
	type op func(Stock, int32) (Stock, error)
	commit := func(s Stock, q int32) (Stock, error) { return s.CommitReservation(q) }
	release := func(s Stock, q int32) (Stock, error) { return s.ReleaseReservation(q) }

	tests := []struct {
		name          string
		do            op
		quantity      int32
		wantErr       error
		wantOnHand    int32
		wantReserved  int32
		wantAvailable int32
	}{
		// Start: 10 on hand, 4 reserved, 6 available.
		{"commit all reserved", commit, 4, nil, 6, 0, 6},
		{"commit one more than reserved", commit, 5, ErrInsufficientReservedStock, 10, 4, 6},
		{"commit one", commit, 1, nil, 9, 3, 6},
		{"commit zero", commit, 0, ErrInvalidReservationQuantity, 10, 4, 6},
		{"release all reserved", release, 4, nil, 10, 0, 10},
		{"release one more than reserved", release, 5, ErrInsufficientReservedStock, 10, 4, 6},
		{"release one", release, 1, nil, 10, 3, 7},
		{"release zero", release, 0, ErrInvalidReservationQuantity, 10, 4, 6},
		{"release negative", release, -1, ErrInvalidReservationQuantity, 10, 4, 6},
		{"commit negative", commit, -1, ErrInvalidReservationQuantity, 10, 4, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.do(mustStock(t, 10, 4), tt.quantity)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got.QuantityOnHand != tt.wantOnHand || got.QuantityReserved != tt.wantReserved {
				t.Fatalf("stock = %+v, want on hand %d, reserved %d", got, tt.wantOnHand, tt.wantReserved)
			}
			if a := got.QuantityAvailable(); a != tt.wantAvailable {
				t.Fatalf("available = %d, want %d", a, tt.wantAvailable)
			}
		})
	}
}

func TestReservationLifecycleNeverOversells(t *testing.T) {
	s := mustStock(t, 2, 0)
	s, err := s.Reserve(1)
	if err != nil {
		t.Fatal(err)
	}
	s, err = s.Reserve(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reserve(1); !errors.Is(err, ErrInsufficientAvailableStock) {
		t.Fatalf("third reserve on 2 units: error = %v", err)
	}
	s, err = s.CommitReservation(1)
	if err != nil {
		t.Fatal(err)
	}
	s, err = s.ReleaseReservation(1)
	if err != nil {
		t.Fatal(err)
	}
	if s.QuantityOnHand != 1 || s.QuantityReserved != 0 || s.QuantityAvailable() != 1 {
		t.Fatalf("after reserve, reserve, commit, release: %+v, want 1 on hand and available", s)
	}
}
