package resilience

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

// RetryConfig holds the retry settings. Zero fields use the defaults.
type RetryConfig struct {
	// MaxAttempts is the total number of tries, including the first. Default 3.
	MaxAttempts int
	// BaseDelay is the wait before the first retry. It doubles each time. Default 100ms.
	BaseDelay time.Duration
	// MaxDelay caps the wait between tries. Default 2s.
	MaxDelay time.Duration
	// Retryable says which errors are worth retrying. Default: every error
	// except context cancellation, context deadline and ErrOpen.
	Retryable func(error) bool
	// Sleep waits between tries. Default is a timer that stops when ctx ends. Tests replace it.
	Sleep func(ctx context.Context, d time.Duration) error
	// Jitter picks the real wait from the backoff delay. Default is a random
	// value between half the delay and the full delay. Tests replace it.
	Jitter func(d time.Duration) time.Duration
}

// Retry calls fn until it succeeds, returns an error that is not retryable,
// the attempts run out, or ctx ends. It returns the last error from fn.
func Retry(ctx context.Context, cfg RetryConfig, fn func(context.Context) error) error {
	cfg = cfg.withDefaults()

	var err error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err = fn(ctx); err == nil {
			return nil
		}
		if attempt == cfg.MaxAttempts || !cfg.Retryable(err) || ctx.Err() != nil {
			return err
		}
		wait := cfg.Jitter(backoff(attempt, cfg.BaseDelay, cfg.MaxDelay))
		if cfg.Sleep(ctx, wait) != nil {
			return err // ctx ended while waiting
		}
	}
	return err
}

// backoff returns base doubled (attempt-1) times, capped at maxDelay.
func backoff(attempt int, base, maxDelay time.Duration) time.Duration {
	d := base
	for i := 1; i < attempt && d < maxDelay; i++ {
		d *= 2
	}
	return min(d, maxDelay)
}

func (c RetryConfig) withDefaults() RetryConfig {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = 100 * time.Millisecond
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = 2 * time.Second
	}
	if c.Retryable == nil {
		c.Retryable = func(err error) bool {
			return !errors.Is(err, context.Canceled) &&
				!errors.Is(err, context.DeadlineExceeded) &&
				!errors.Is(err, ErrOpen)
		}
	}
	if c.Sleep == nil {
		c.Sleep = sleep
	}
	if c.Jitter == nil {
		c.Jitter = jitter
	}
	return c
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// jitter returns a random duration between d/2 and d, so many clients
// that failed together do not all retry at the same moment.
func jitter(d time.Duration) time.Duration {
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}
