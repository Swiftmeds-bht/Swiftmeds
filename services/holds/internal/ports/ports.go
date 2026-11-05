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
