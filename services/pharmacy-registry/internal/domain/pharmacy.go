// Package domain contains pharmacy registration and verification rules.
package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusVerified  Status = "VERIFIED"
	StatusRejected  Status = "REJECTED"
	StatusSuspended Status = "SUSPENDED"
)

var (
	ErrMissingPharmacyID = errors.New("pharmacy_id is required")
	ErrMissingOwnerID    = errors.New("owner_user_id is required")
	ErrMissingName       = errors.New("name is required")
	ErrMissingLicense    = errors.New("license_number is required")
	ErrInvalidLocation   = errors.New("latitude must be -90..90 and longitude -180..180")
	ErrInvalidTransition = errors.New("invalid verification status transition")
	ErrMissingReason     = errors.New("reason is required for rejection or suspension")
	ErrNotFound          = errors.New("pharmacy not found")
	// ErrLicenseTaken: another pharmacy is registered with this licence number.
	ErrLicenseTaken = errors.New("a pharmacy with this licence number is already registered")
)

type GeoPoint struct {
	Latitude  float64
	Longitude float64
}

func (p GeoPoint) Validate() error {
	if math.IsNaN(p.Latitude) || math.IsNaN(p.Longitude) ||
		p.Latitude < -90 || p.Latitude > 90 || p.Longitude < -180 || p.Longitude > 180 {
		return ErrInvalidLocation
	}
	return nil
}

type OpeningHours struct {
	Day      int32
	OpensAt  string
	ClosesAt string
}

func (h OpeningHours) Validate() error {
	if h.Day < 0 || h.Day > 6 || len(h.OpensAt) != 5 || len(h.ClosesAt) != 5 {
		return errors.New("hours must use day 0..6 and HH:MM times")
	}
	return nil
}

type Pharmacy struct {
	ID            string
	OwnerID       string
	Name          string
	LicenseNumber string
	// Phone and Address are for the administrator who verifies the pharmacy
	// (SCRUM-141). Both are optional.
	Phone        string
	Address      string
	Dzongkhag    string
	Location     GeoPoint
	Hours        []OpeningHours
	Status       Status
	StatusReason string
	CreatedAt    time.Time
	VerifiedAt   *time.Time
}

func NewPharmacy(id, ownerID, name, licenseNumber, dzongkhag string, location GeoPoint, hours []OpeningHours, now time.Time) (Pharmacy, error) {
	switch {
	case id == "":
		return Pharmacy{}, ErrMissingPharmacyID
	case ownerID == "":
		return Pharmacy{}, ErrMissingOwnerID
	case name == "":
		return Pharmacy{}, ErrMissingName
	case licenseNumber == "":
		return Pharmacy{}, ErrMissingLicense
	case location.Validate() != nil:
		return Pharmacy{}, ErrInvalidLocation
	}
	for _, h := range hours {
		if err := h.Validate(); err != nil {
			return Pharmacy{}, fmt.Errorf("invalid hours: %w", err)
		}
	}
	return Pharmacy{
		ID: id, OwnerID: ownerID, Name: name, LicenseNumber: licenseNumber, Dzongkhag: strings.TrimSpace(dzongkhag),
		Location: location, Hours: append([]OpeningHours(nil), hours...),
		Status: StatusPending, CreatedAt: now,
	}, nil
}

func ValidateTransition(from, to Status) error {
	allowed := from == StatusPending && (to == StatusVerified || to == StatusRejected)
	allowed = allowed || from == StatusVerified && to == StatusSuspended
	if !allowed {
		return fmt.Errorf("%w: %s to %s", ErrInvalidTransition, from, to)
	}
	return nil
}

func (p Pharmacy) ApplyStatus(to Status, reason string, now time.Time) (Pharmacy, error) {
	if err := ValidateTransition(p.Status, to); err != nil {
		return Pharmacy{}, err
	}
	if (to == StatusRejected || to == StatusSuspended) && reason == "" {
		return Pharmacy{}, ErrMissingReason
	}
	p.Status, p.StatusReason = to, reason
	if to == StatusVerified {
		p.VerifiedAt = &now
	}
	return p, nil
}
