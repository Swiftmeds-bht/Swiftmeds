package domain

import (
	"errors"
	"testing"
	"time"
)

func TestVerificationTransitions(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
		ok   bool
	}{
		{"pending to verified", StatusPending, StatusVerified, true},
		{"pending to rejected", StatusPending, StatusRejected, true},
		{"pending to suspended", StatusPending, StatusSuspended, false},
		{"verified to suspended", StatusVerified, StatusSuspended, true},
		{"verified to rejected", StatusVerified, StatusRejected, false},
		{"rejected to verified", StatusRejected, StatusVerified, false},
		{"rejected to rejected", StatusRejected, StatusRejected, false},
		{"suspended to verified", StatusSuspended, StatusVerified, true},
		{"rejected to pending", StatusRejected, StatusPending, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransition(tt.from, tt.to)
			if (err == nil) != tt.ok {
				t.Fatalf("ValidateTransition(%s, %s) = %v, want allowed=%v", tt.from, tt.to, err, tt.ok)
			}
		})
	}
}

func TestNewPharmacyStartsPending(t *testing.T) {
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	p, err := NewPharmacy("ph-1", "owner-1", "Paro Pharmacy", "LIC-1",
		"  Paro  ",
		GeoPoint{Latitude: 27.43, Longitude: 89.41},
		[]OpeningHours{{Day: 1, OpensAt: "09:00", ClosesAt: "17:00"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusPending {
		t.Fatalf("status = %s, want PENDING", p.Status)
	}
	if p.Dzongkhag != "Paro" {
		t.Fatalf("dzongkhag = %q, want %q", p.Dzongkhag, "Paro")
	}
	if p.VerifiedAt != nil {
		t.Fatal("new pharmacy has verified_at")
	}
}

func TestNewPharmacyRejectsInvalidInput(t *testing.T) {
	_, err := NewPharmacy("", "owner-1", "Name", "LIC-1", "", GeoPoint{}, nil, time.Now())
	if !errors.Is(err, ErrMissingPharmacyID) {
		t.Fatalf("err = %v, want %v", err, ErrMissingPharmacyID)
	}
	_, err = NewPharmacy("ph-1", "owner-1", "Name", "LIC-1", "", GeoPoint{Latitude: 91}, nil, time.Now())
	if !errors.Is(err, ErrInvalidLocation) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidLocation)
	}
}

func TestOpeningHoursValidation(t *testing.T) {
	tests := []struct {
		name  string
		hours OpeningHours
		valid bool
	}{
		{"valid", OpeningHours{Day: 0, OpensAt: "00:00", ClosesAt: "23:59"}, true},
		{"negative day", OpeningHours{Day: -1, OpensAt: "09:00", ClosesAt: "17:00"}, false},
		{"day too high", OpeningHours{Day: 7, OpensAt: "09:00", ClosesAt: "17:00"}, false},
		{"invalid open time", OpeningHours{Day: 1, OpensAt: "9:00", ClosesAt: "17:00"}, false},
		{"invalid close time", OpeningHours{Day: 1, OpensAt: "09:00", ClosesAt: "24:00"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.hours.Validate()
			if (err == nil) != tt.valid {
				t.Fatalf("Validate() = %v, want valid=%v", err, tt.valid)
			}
		})
	}
}
