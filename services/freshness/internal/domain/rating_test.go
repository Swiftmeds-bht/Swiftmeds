package domain

import (
	"errors"
	"testing"
)

func TestNewKeyValidates(t *testing.T) {
	cases := []struct {
		name                  string
		pharmacyID, productID string
		wantErr               error
	}{
		{"valid", "ph-1", "med-1", nil},
		{"missing pharmacy", "", "med-1", ErrMissingPharmacyID},
		{"missing product", "ph-1", "", ErrMissingProductID},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key, err := NewKey(c.pharmacyID, c.productID)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if err == nil && (key.PharmacyID != "ph-1" || key.ProductID != "med-1") {
				t.Errorf("key = %+v, fields were not kept", key)
			}
		})
	}
}
