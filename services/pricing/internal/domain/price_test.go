package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewPrice(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

	// Boundary values for the amount: 0 and -1 are rejected, 1 is the smallest valid price.
	cases := []struct {
		name     string
		pharmacy string
		product  string
		amount   int64
		wantErr  error
	}{
		{"valid", "ph-1", "pr-1", 4500, nil},
		{"smallest valid amount", "ph-1", "pr-1", 1, nil},
		{"zero amount", "ph-1", "pr-1", 0, ErrNonPositivePrice},
		{"negative amount", "ph-1", "pr-1", -1, ErrNonPositivePrice},
		{"missing pharmacy", "", "pr-1", 4500, ErrMissingPharmacyID},
		{"missing product", "ph-1", "", 4500, ErrMissingProductID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewPrice(tc.pharmacy, tc.product, tc.amount, now)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			want := Price{Key: Key{tc.pharmacy, tc.product}, AmountChhertum: tc.amount, UpdatedAt: now}
			if p != want {
				t.Errorf("price = %+v, want %+v", p, want)
			}
		})
	}
}
