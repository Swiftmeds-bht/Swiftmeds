package postgres

import (
	"testing"
	"time"
)

type pharmacyRow struct{}

func (pharmacyRow) Scan(dest ...any) error {
	*dest[0].(*string) = "ph-1"
	*dest[1].(*string) = "owner-1"
	*dest[2].(*string) = "Paro Pharmacy"
	*dest[3].(*string) = "LIC-1"
	*dest[4].(*string) = "Paro"
	*dest[5].(*float64) = 27.43
	*dest[6].(*float64) = 89.41
	*dest[7].(*[]byte) = []byte(`[]`)
	*dest[8].(*string) = "VERIFIED"
	*dest[9].(*string) = ""
	*dest[10].(*time.Time) = time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	*dest[11].(**time.Time) = nil
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
