// Package domain holds Watchlist's rules: what a valid subscription is and
// which stock events can trigger an alert. No I/O here.
package domain

import (
	"errors"
	"strings"
	"time"
)

const (
	// The radius limits match Geo and Search, so a subscription can always be checked.
	MinRadiusM = 1
	MaxRadiusM = 50_000
	// DefaultRadiusM is used when the request leaves radius_m at 0 (the patient app's default too).
	DefaultRadiusM = 5_000
)

var (
	ErrMissingUserID         = errors.New("user_id is required")
	ErrMissingProductID      = errors.New("product_id is required")
	ErrMissingPharmacyID     = errors.New("pharmacy_id is required")
	ErrMissingSubscriptionID = errors.New("subscription_id is required")
	ErrInvalidLocation       = errors.New("latitude must be -90..90 and longitude -180..180")
	ErrInvalidRadius         = errors.New("radius_m must be between 1 and 50000")
	// ErrNotFound covers both "no such subscription" and "not yours", so a
	// caller cannot probe for other users' subscription IDs.
	ErrNotFound = errors.New("subscription not found")
)

// Point is a WGS84 coordinate in degrees.
type Point struct {
	Lat, Lng float64
}

func (p Point) valid() bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lng >= -180 && p.Lng <= 180
}

// Subscription is "tell me when ProductID is in stock within RadiusM of Location".
type Subscription struct {
	ID        string
	UserID    string
	ProductID string
	Location  Point
	RadiusM   int32
	CreatedAt time.Time
	// NotifiedAt is set once an alert fired. A notified subscription never fires again;
	// the patient subscribes again if they want another alert.
	NotifiedAt time.Time
}

// Notified reports whether this subscription has already produced its alert.
func (s Subscription) Notified() bool { return !s.NotifiedAt.IsZero() }

// NewSubscription validates a request. IDs are trimmed; radius 0 means DefaultRadiusM.
func NewSubscription(id, userID, productID string, at Point, radiusM int32, now time.Time) (Subscription, error) {
	userID, productID = strings.TrimSpace(userID), strings.TrimSpace(productID)
	if userID == "" {
		return Subscription{}, ErrMissingUserID
	}
	if productID == "" {
		return Subscription{}, ErrMissingProductID
	}
	if !at.valid() {
		return Subscription{}, ErrInvalidLocation
	}
	if radiusM == 0 {
		radiusM = DefaultRadiusM
	}
	if radiusM < MinRadiusM || radiusM > MaxRadiusM {
		return Subscription{}, ErrInvalidRadius
	}
	return Subscription{
		ID:        id,
		UserID:    userID,
		ProductID: productID,
		Location:  at,
		RadiusM:   radiusM,
		CreatedAt: now,
	}, nil
}

// StockIncrease is the part of the stock.increased event Watchlist uses.
type StockIncrease struct {
	PharmacyID   string
	ProductID    string
	NewAvailable int32
	OccurredAt   time.Time
}

// Validate rejects events that cannot be matched to anything.
func (e StockIncrease) Validate() error {
	if strings.TrimSpace(e.PharmacyID) == "" {
		return ErrMissingPharmacyID
	}
	if strings.TrimSpace(e.ProductID) == "" {
		return ErrMissingProductID
	}
	return nil
}

// InStock reports whether the pharmacy now has units a patient could reserve.
func (e StockIncrease) InStock() bool { return e.NewAvailable > 0 }

// Alert records that a subscriber's medicine is back in stock at a pharmacy
// within their radius. StreamAlerts (SCRUM-71) delivers these to the patient app.
type Alert struct {
	ID                string
	SubscriptionID    string
	UserID            string
	ProductID         string
	PharmacyID        string
	QuantityAvailable int32
	OccurredAt        time.Time
}
