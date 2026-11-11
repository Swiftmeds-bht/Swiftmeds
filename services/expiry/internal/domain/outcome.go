package domain

import "errors"

// Outcome records how a deadline was settled, so a deadline is settled once.
type Outcome string

const (
	// OutcomeExpired means the hold was ACTIVE and is now EXPIRED.
	OutcomeExpired Outcome = "EXPIRED"
	// OutcomeAlreadyFinal means the hold had already been collected, cancelled
	// or rejected, so there was nothing to expire.
	OutcomeAlreadyFinal Outcome = "ALREADY_FINAL"
	// OutcomeNotFound means the Holds service has no such hold.
	OutcomeNotFound Outcome = "NOT_FOUND"
)

// HoldStatus is the status the Holds service reports for a hold.
type HoldStatus string

const (
	HoldPending   HoldStatus = "PENDING"
	HoldActive    HoldStatus = "ACTIVE"
	HoldCollected HoldStatus = "COLLECTED"
	HoldExpired   HoldStatus = "EXPIRED"
	HoldCancelled HoldStatus = "CANCELLED"
	HoldRejected  HoldStatus = "REJECTED"
)

// Final reports whether the hold can no longer change.
func (s HoldStatus) Final() bool {
	switch s {
	case HoldCollected, HoldExpired, HoldCancelled, HoldRejected:
		return true
	}
	return false
}

// Errors a HoldExpirer returns. The app layer decides what each one means for the sweep.
var (
	// ErrHoldNotFound means the Holds service has no such hold.
	ErrHoldNotFound = errors.New("hold not found in the Holds service")
	// ErrNotYetExpired means Holds says the hold's time has not run out yet.
	// The two services' clocks can differ by a moment.
	ErrNotYetExpired = errors.New("hold has not reached its expiry time yet")
	// ErrHoldsUnavailable means the Holds service could not be reached in time.
	ErrHoldsUnavailable = errors.New("holds service unavailable")
)
