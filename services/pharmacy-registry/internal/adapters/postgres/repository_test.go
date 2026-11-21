package postgres

import (
	"testing"
	"time"
)

type pharmacyRow struct{}

// Scan fills the columns in pharmacySelect's order.
func (pharmacyRow) Scan(dest ...any) error {
	*dest[0].(*string) = "ph-1"
	*dest[1].(*string) = "owner-1"
	*dest[2].(*string) = "Paro Pharmacy"
	*dest[3].(*string) = "LIC-1"
	*dest[4].(*string) = "+975 8 271 234"
	*dest[5].(*string) = "Main Street, Paro"
	*dest[6].(*string) = "Paro"
	*dest[7].(*float64) = 27.43
	*dest[8].(*float64) = 89.41
	*dest[9].(*[]byte) = []byte(`[]`)
	*dest[10].(*string) = "VERIFIED"
	*dest[11].(*string) = ""
	*dest[12].(*time.Time) = time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	*dest[13].(**time.Time) = nil
	return nil
}

func TestScanRowPreservesDzongkhag(t *testing.T) {
	p, err := scanPharmacy(pharmacyRow{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Dzongkhag != "Paro" {
		t.Fatalf("dzongkhag = %q, want %q", p.Dzongkhag, "Paro")
	}
}

func TestScanRowReadsPhoneAndAddress(t *testing.T) {
	p, err := scanPharmacy(pharmacyRow{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Phone != "+975 8 271 234" || p.Address != "Main Street, Paro" {
		t.Fatalf("phone=%q address=%q", p.Phone, p.Address)
	}
}
