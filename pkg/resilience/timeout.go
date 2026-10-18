package resilience

import (
	"context"
	"time"
)

// WithTimeout runs fn with a context that ends after d.
// fn must stop when its context ends; gRPC calls do this automatically.
// A d of zero or less means no extra timeout.
func WithTimeout(ctx context.Context, d time.Duration, fn func(context.Context) error) error {
	if d <= 0 {
		return fn(ctx)
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return fn(ctx)
}
