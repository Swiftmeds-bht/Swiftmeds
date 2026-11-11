// Package domain holds the Expiry rules. It uses the standard library only.
package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrMissingHoldID = errors.New("hold_id is required")
	ErrMissingExpiry = errors.New("expires_at is required")
)

// Deadline is the moment a confirmed hold runs out. Expiry stores one per hold
// when it learns of the hold, and the sweep expires the hold once it is due.
type Deadline struct {
	HoldID    string
	ExpiresAt time.Time
}

// NewDeadline validates a deadline. The time is kept in UTC.
func NewDeadline(holdID string, expiresAt time.Time) (Deadline, error) {
	if strings.TrimSpace(holdID) == "" {
		return Deadline{}, ErrMissingHoldID
	}
	if expiresAt.IsZero() {
		return Deadline{}, ErrMissingExpiry
	}
	return Deadline{HoldID: holdID, ExpiresAt: expiresAt.UTC()}, nil
}

// Due reports whether the deadline has been reached at asOf. A hold that
// expires exactly at asOf is due: "expires at 10:00" means it is over at 10:00,
// not one tick later.
func (d Deadline) Due(asOf time.Time) bool {
	return !asOf.Before(d.ExpiresAt)
}
