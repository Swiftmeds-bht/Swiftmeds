package domain

import (
	"errors"
	"time"
)

var (
	ErrMissingHoldID     = errors.New("hold_id is required")
	ErrMissingUserID     = errors.New("user_id is required")
	ErrMissingPharmacyID = errors.New("pharmacy_id is required")
	ErrMissingProductID  = errors.New("product_id is required")
	ErrInvalidQuantity   = errors.New("quantity must be between 1 and 5")
	ErrInvalidDuration   = errors.New("hold duration must be greater than zero")
)

// A hold can reserve between MinQuantity and MaxQuantity packs.
const (
	MinQuantity = 1
	MaxQuantity = 5
)

// Hold is a short reservation of stock for one patient.
// Pickup codes are added in SCRUM-53.
type Hold struct {
	ID              string
	UserID          string
	PharmacyID      string
	ProductID       string
	Quantity        int
	Status          Status
	CreatedAt       time.Time
	ExpiresAt       time.Time // zero until the hold becomes ACTIVE
	StatusChangedAt time.Time // when the hold reached its current status
	StatusReason    string    // set for REJECTED and CANCELLED
}

// NewHold validates and builds a PENDING hold.
func NewHold(id, userID, pharmacyID, productID string, quantity int, now time.Time) (Hold, error) {
	switch {
	case id == "":
		return Hold{}, ErrMissingHoldID
	case userID == "":
		return Hold{}, ErrMissingUserID
	case pharmacyID == "":
		return Hold{}, ErrMissingPharmacyID
	case productID == "":
		return Hold{}, ErrMissingProductID
	case quantity < MinQuantity || quantity > MaxQuantity:
		return Hold{}, ErrInvalidQuantity
	}
	return Hold{
		ID:              id,
		UserID:          userID,
		PharmacyID:      pharmacyID,
		ProductID:       productID,
		Quantity:        quantity,
		Status:          Pending,
		CreatedAt:       now,
		StatusChangedAt: now,
	}, nil
}

// Each method below returns a changed copy and leaves the original hold alone.
// If the event is not allowed in the current status, it returns an empty Hold
// and ErrInvalidTransition.

// Reserve is the stock.reserved event: PENDING -> ACTIVE. The hold expires
// holdFor after now, so the expiry time is fixed here and not before.
func (h Hold) Reserve(now time.Time, holdFor time.Duration) (Hold, error) {
	moved, err := h.move(StockReserved, now)
	if err != nil {
		return Hold{}, err
	}
	if holdFor <= 0 {
		return Hold{}, ErrInvalidDuration
	}
	moved.ExpiresAt = now.Add(holdFor)
	return moved, nil
}

// Reject is the stock.unavailable event: PENDING -> REJECTED.
func (h Hold) Reject(now time.Time, reason string) (Hold, error) {
	moved, err := h.move(StockUnavailable, now)
	if err != nil {
		return Hold{}, err
	}
	moved.StatusReason = reason
	return moved, nil
}

// Collect is the collect event: ACTIVE -> COLLECTED.
func (h Hold) Collect(now time.Time) (Hold, error) {
	return h.move(Collect, now)
}

// Expire is the expire event: ACTIVE -> EXPIRED.
func (h Hold) Expire(now time.Time) (Hold, error) {
	return h.move(Expire, now)
}

// Cancel is the cancel event: PENDING or ACTIVE -> CANCELLED.
func (h Hold) Cancel(now time.Time, reason string) (Hold, error) {
	moved, err := h.move(Cancel, now)
	if err != nil {
		return Hold{}, err
	}
	moved.StatusReason = reason
	return moved, nil
}

// move returns a copy of the hold in the status the event leads to.
func (h Hold) move(event Event, now time.Time) (Hold, error) {
	to, err := Next(h.Status, event)
	if err != nil {
		return Hold{}, err
	}
	h.Status = to
	h.StatusChangedAt = now
	return h, nil
}
