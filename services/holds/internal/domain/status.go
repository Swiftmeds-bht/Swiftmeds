// Package domain holds Holds' entities and business rules.
// It imports nothing outside the standard library.
package domain

import "errors"

// ErrInvalidTransition is returned when an event is not allowed in the hold's current status.
var ErrInvalidTransition = errors.New("invalid hold status transition")

// Status is where a hold is in its life.
type Status string

const (
	Pending   Status = "PENDING"   // waiting for Inventory to answer
	Active    Status = "ACTIVE"    // stock is reserved for the patient
	Collected Status = "COLLECTED" // the patient picked it up
	Expired   Status = "EXPIRED"   // the time ran out
	Cancelled Status = "CANCELLED" // the patient cancelled
	Rejected  Status = "REJECTED"  // Inventory had no stock
)

// Event is something that happens to a hold.
type Event string

const (
	StockReserved    Event = "stock.reserved"
	StockUnavailable Event = "stock.unavailable"
	Collect          Event = "collect"
	Expire           Event = "expire"
	Cancel           Event = "cancel"
)

// transitions is the whole state machine: the status each event leads to.
// A status or event that is not in the table is not allowed, so COLLECTED,
// EXPIRED, CANCELLED and REJECTED (which have no row) are final.
var transitions = map[Status]map[Event]Status{
	Pending: {StockReserved: Active, StockUnavailable: Rejected, Cancel: Cancelled},
	Active:  {Collect: Collected, Expire: Expired, Cancel: Cancelled},
}

// Next returns the status that event leads to from the given status,
// or ErrInvalidTransition if the event is not allowed there.
func Next(from Status, event Event) (Status, error) {
	to, ok := transitions[from][event]
	if !ok {
		return "", ErrInvalidTransition
	}
	return to, nil
}
