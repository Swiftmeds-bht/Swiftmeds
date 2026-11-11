// Package ports defines the interfaces the app layer needs from the outside world.
// Adapters (memory, postgres, ...) implement them; tests use fakes.
package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// HoldRepository stores holds. Every implementation must pass the shared
// contract tests in internal/adapters/repotest.
type HoldRepository interface {
	// Create stores a new hold. It returns domain.ErrAlreadyExists if the ID is
	// taken. If maxOpen is above zero, it also returns domain.ErrLimitReached
	// when the patient already has maxOpen holds that are PENDING or ACTIVE.
	// The count and the insert are one step, so many creates for the same
	// patient at the same moment cannot all pass the check.
	Create(ctx context.Context, h domain.Hold, maxOpen int) error
	// Get returns the hold, or domain.ErrNotFound.
	Get(ctx context.Context, id string) (domain.Hold, error)
	// Update replaces the stored hold with h, but only if the stored status is
	// still from. It returns domain.ErrNotFound if there is no such hold, and
	// domain.ErrConflict if the status has changed since the caller read it
	// (for example, Collect and Expire at the same moment: one wins, one gets the conflict).
	// An ACTIVE hold's pickup code must be unique among the ACTIVE holds of its
	// pharmacy: otherwise Update returns domain.ErrPickupCodeTaken and stores nothing.
	Update(ctx context.Context, h domain.Hold, from domain.Status) error
	// List returns the holds that match the filter, newest first (by created_at,
	// then by ID), starting after f.After and at most f.Limit of them.
	List(ctx context.Context, f domain.ListFilter) ([]domain.Hold, error)
}

// Inventory is the part of the Inventory service that Holds calls. Both calls
// are safe to repeat for the same hold: Inventory keeps one reservation per hold ID.
type Inventory interface {
	// Release gives the hold's reserved units back to available stock
	// (the hold was cancelled or expired). Any failure is domain.ErrInventoryUnavailable.
	Release(ctx context.Context, holdID string) error
	// Commit makes the reserved units leave stock for good (the hold was
	// collected). Any failure is domain.ErrInventoryUnavailable.
	Commit(ctx context.Context, holdID string) error
}

// Events publishes the reservation saga's events from Holds. A hold's events
// always carry the same event ID, so sending one twice is harmless: listeners
// ignore the second copy.
type Events interface {
	// HoldRequested says a PENDING hold wants stock (routing key hold.requested).
	HoldRequested(ctx context.Context, h domain.Hold) error
	// HoldConfirmed says the hold is ACTIVE, with its pickup code and expiry
	// time (routing key hold.confirmed).
	HoldConfirmed(ctx context.Context, h domain.Hold) error
}
