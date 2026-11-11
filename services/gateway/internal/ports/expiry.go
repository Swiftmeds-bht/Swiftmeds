package ports

import (
	"context"
	"time"
)

// HoldExpirySweeper asks the Expiry service to expire the holds whose time has run out.
type HoldExpirySweeper interface {
	// RunSweep expires every hold that is due at asOf and returns their IDs.
	// The list is empty, never nil, when no hold was due. Calling it again is safe.
	RunSweep(ctx context.Context, asOf time.Time) ([]string, error)
}
