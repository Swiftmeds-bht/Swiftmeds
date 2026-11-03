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
	// Create stores a new hold. It returns domain.ErrAlreadyExists if the ID is taken.
	Create(ctx context.Context, h domain.Hold) error
	// Get returns the hold, or domain.ErrNotFound.
	Get(ctx context.Context, id string) (domain.Hold, error)
	// Update replaces the stored hold with h, but only if the stored status is
	// still from. It returns domain.ErrNotFound if there is no such hold, and
	// domain.ErrConflict if the status has changed since the caller read it
	// (for example, Collect and Expire at the same moment: one wins, one gets the conflict).
	Update(ctx context.Context, h domain.Hold, from domain.Status) error
}
