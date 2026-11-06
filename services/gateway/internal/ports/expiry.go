package ports

import (
	"context"
	"time"
)

// HoldSweeper asks the Expiry service to expire every hold whose deadline has
// passed.
type HoldSweeper interface {
	// RunSweep expires the holds due at asOf and returns their IDs. It is safe
	// to call again: holds that are already expired are not listed twice.
	RunSweep(ctx context.Context, asOf time.Time) ([]string, error)
}
