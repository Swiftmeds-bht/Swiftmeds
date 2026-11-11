// Package ports defines the interfaces the app layer needs from the outside world.
// Adapters (memory, postgres, the Holds client, ...) implement them; tests use fakes.
package ports

import (
	"context"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
)

// DeadlineRepository stores the deadlines of confirmed holds. Every
// implementation must pass the shared contract tests in internal/adapters/repotest.
type DeadlineRepository interface {
	// Save stores a hold's deadline. A hold that is already stored keeps its
	// first deadline and gives no error, so a redelivered event is harmless.
	Save(ctx context.Context, d domain.Deadline) error

	// ListDue returns pending deadlines that are due at asOf (expires_at <= asOf,
	// so a deadline exactly at asOf is included), ordered by expires_at and then
	// hold ID. When after is not nil, only deadlines after it in that order are
	// returned, so a caller can page through them. At most limit are returned.
	ListDue(ctx context.Context, asOf time.Time, after *domain.Deadline, limit int) ([]domain.Deadline, error)

	// Complete settles a pending deadline. It returns true only for the one call
	// that settled it: false means the deadline was already settled or is unknown.
	Complete(ctx context.Context, holdID string, outcome domain.Outcome, at time.Time) (bool, error)
}

// HoldExpirer asks the Holds service to expire a hold.
type HoldExpirer interface {
	// ExpireHold returns the status the hold has afterwards. Holds does nothing to
	// a hold that is not ACTIVE and returns its current status. It returns
	// domain.ErrHoldNotFound, domain.ErrNotYetExpired or domain.ErrHoldsUnavailable
	// for the cases those errors describe.
	ExpireHold(ctx context.Context, holdID string) (domain.HoldStatus, error)
}
