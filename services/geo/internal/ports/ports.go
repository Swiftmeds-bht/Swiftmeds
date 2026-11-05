// Package ports defines what Geo's app layer needs from the outside world.
package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/domain"
)

// LocationRepository stores the locations of verified pharmacies. Every
// implementation must pass the contract tests in internal/adapters/repotest.
type LocationRepository interface {
	// Upsert inserts or moves a pharmacy's location.
	Upsert(ctx context.Context, loc domain.Location) error
	// Remove deletes a pharmacy's location. Removing an unknown pharmacy is not an error.
	Remove(ctx context.Context, pharmacyID string) error
	// WithinBox returns every location inside the box (edges included), in any order.
	WithinBox(ctx context.Context, box domain.Box) ([]domain.Location, error)
}
