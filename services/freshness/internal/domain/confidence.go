// Package domain holds Freshness's entities and business rules.
// It imports nothing outside the standard library.
package domain

import (
	"errors"
	"time"
)

var (
	ErrMissingPharmacyID  = errors.New("pharmacy_id is required")
	ErrMissingProductID   = errors.New("product_id is required")
	ErrMissingConfirmedAt = errors.New("confirmed_at is required")
	ErrInvalidThresholds  = errors.New("thresholds must be greater than zero, and high must be shorter than medium")
)

// Confidence says how much to trust a stock figure, based on how old its last confirmation is.
type Confidence string

const (
	High   Confidence = "HIGH"
	Medium Confidence = "MEDIUM"
	Low    Confidence = "LOW"
)

// Score is the number the proto sends to other services (0.0 to 1.0).
func (c Confidence) Score() float64 {
	switch c {
	case High:
		return 1.0
	case Medium:
		return 0.6
	default:
		return 0.2
	}
}

// Thresholds are the ages where confidence drops.
type Thresholds struct {
	HighWithin   time.Duration // younger than this is HIGH
	MediumWithin time.Duration // younger than this is MEDIUM, older is LOW
}

// DefaultThresholds is HIGH under 6 hours and MEDIUM under 24 hours.
func DefaultThresholds() Thresholds {
	return Thresholds{HighWithin: 6 * time.Hour, MediumWithin: 24 * time.Hour}
}

// NewThresholds checks that 0 < high < medium.
func NewThresholds(high, medium time.Duration) (Thresholds, error) {
	if high <= 0 || medium <= high {
		return Thresholds{}, ErrInvalidThresholds
	}
	return Thresholds{HighWithin: high, MediumWithin: medium}, nil
}

// ConfidenceFor returns the confidence for a confirmation of the given age.
// An age exactly on a threshold belongs to the lower level.
func (t Thresholds) ConfidenceFor(age time.Duration) Confidence {
	switch {
	case age < t.HighWithin:
		return High
	case age < t.MediumWithin:
		return Medium
	default:
		return Low
	}
}

// Key identifies one product at one pharmacy.
type Key struct {
	PharmacyID string
	ProductID  string
}

// Confirmation records when a stock row was last confirmed.
type Confirmation struct {
	Key
	ConfirmedAt time.Time
}

// NewConfirmation validates and builds a Confirmation.
func NewConfirmation(pharmacyID, productID string, confirmedAt time.Time) (Confirmation, error) {
	switch {
	case pharmacyID == "":
		return Confirmation{}, ErrMissingPharmacyID
	case productID == "":
		return Confirmation{}, ErrMissingProductID
	case confirmedAt.IsZero():
		return Confirmation{}, ErrMissingConfirmedAt
	}
	return Confirmation{
		Key:         Key{PharmacyID: pharmacyID, ProductID: productID},
		ConfirmedAt: confirmedAt,
	}, nil
}

// Confidence works out the confidence as of now.
func (c Confirmation) Confidence(t Thresholds, now time.Time) Confidence {
	return t.ConfidenceFor(now.Sub(c.ConfirmedAt))
}
