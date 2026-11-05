// Package ports holds the interfaces the gateway needs from outside itself.
package ports

import (
	"context"
	"time"
)

// DecayApplier asks the Freshness service to recompute every stored stock
// confidence as of a given time.
type DecayApplier interface {
	// ApplyDecay returns how many stock rows changed confidence.
	ApplyDecay(ctx context.Context, asOf time.Time) (int, error)
}
