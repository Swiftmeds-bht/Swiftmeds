// Package ports defines the interfaces the app layer needs from the outside world.
// Adapters (postgres, memory) implement them; tests use fakes.
package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
)

// TrustRepository stores one domain.Record per pharmacy. Every implementation
// must pass the contract tests in internal/adapters/repotest.
type TrustRepository interface {
	// ApplyFeedback loads the pharmacy's record (an empty one if it has none),
	// replaces it with update(record) and remembers f.ID, as one atomic step.
	// If f.ID was applied before it changes nothing and returns false, so a
	// redelivered or republished event cannot count twice.
	ApplyFeedback(ctx context.Context, f domain.Feedback, update func(domain.Record) domain.Record) (applied bool, err error)
	// Records returns the stored records of the given pharmacies. Pharmacies
	// without feedback are simply missing from the map.
	Records(ctx context.Context, pharmacyIDs []string) (map[string]domain.Record, error)
}
