package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

// recorder replaces Sleep so tests run instantly and can check the waits.
type recorder struct{ waits []time.Duration }

func (r *recorder) Sleep(_ context.Context, d time.Duration) error {
	r.waits = append(r.waits, d)
	return nil
}

func noJitter(d time.Duration) time.Duration { return d }

func testRetryConfig(r *recorder, attempts int) RetryConfig {
	return RetryConfig{
		MaxAttempts: attempts,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    350 * time.Millisecond,
		Sleep:       r.Sleep,
		Jitter:      noJitter,
	}
}

func TestRetryStopsAfterMaxAttempts(t *testing.T) {
	r := &recorder{}
	calls := 0

	err := Retry(context.Background(), testRetryConfig(r, 3), func(context.Context) error {
		calls++
		return errBoom
	})

	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the last error", err)
	}
	if len(r.waits) != 2 {
		t.Fatalf("waits = %d, want 2 (none after the last attempt)", len(r.waits))
	}
}

func TestRetryReturnsNilOnceACallSucceeds(t *testing.T) {
	r := &recorder{}
	calls := 0

	err := Retry(context.Background(), testRetryConfig(r, 5), func(context.Context) error {
		calls++
		if calls < 3 {
			return errBoom
		}
		return nil
	})

	if err != nil || calls != 3 {
		t.Fatalf("got (err=%v, calls=%d), want (nil, 3)", err, calls)
	}
}

func TestRetryNeverRetriesNonRetryableErrors(t *testing.T) {
	r := &recorder{}
	cfg := testRetryConfig(r, 5)
	cfg.Retryable = func(error) bool { return false }
	calls := 0

	err := Retry(context.Background(), cfg, func(context.Context) error {
		calls++
		return errBoom
	})

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
	if len(r.waits) != 0 {
		t.Fatalf("waited %d times, want 0", len(r.waits))
	}
}

func TestRetryBackoffDoublesThenStopsAtMaxDelay(t *testing.T) {
	r := &recorder{}

	_ = Retry(context.Background(), testRetryConfig(r, 5), func(context.Context) error { return errBoom })

	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 350 * time.Millisecond, 350 * time.Millisecond}
	if len(r.waits) != len(want) {
		t.Fatalf("waits = %v, want %v", r.waits, want)
	}
	for i := range want {
		if r.waits[i] != want[i] {
			t.Errorf("wait %d = %v, want %v", i, r.waits[i], want[i])
		}
	}
}

func TestBackoff(t *testing.T) {
	base, maxDelay := 100*time.Millisecond, time.Second
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, time.Second},
		{50, time.Second},
	}
	for _, c := range cases {
		if got := backoff(c.attempt, base, maxDelay); got != c.want {
			t.Errorf("backoff(%d) = %v, want %v", c.attempt, got, c.want)
		}
	}
}

func TestRetryUsesTheJitteredDelay(t *testing.T) {
	r := &recorder{}
	cfg := testRetryConfig(r, 2)
	cfg.Jitter = func(d time.Duration) time.Duration { return d / 2 }

	_ = Retry(context.Background(), cfg, func(context.Context) error { return errBoom })

	if len(r.waits) != 1 || r.waits[0] != 50*time.Millisecond {
		t.Fatalf("waits = %v, want [50ms]", r.waits)
	}
}

func TestDefaultJitterStaysBetweenHalfAndFullDelay(t *testing.T) {
	d := 400 * time.Millisecond
	for range 1000 {
		got := jitter(d)
		if got < d/2 || got > d {
			t.Fatalf("jitter(%v) = %v, want between %v and %v", d, got, d/2, d)
		}
	}
}

func TestRetryStopsWhenContextEnds(t *testing.T) {
	r := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	err := Retry(ctx, testRetryConfig(r, 5), func(context.Context) error {
		calls++
		cancel()
		return errBoom
	})

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestRetryStopsWhenContextEndsWhileWaiting(t *testing.T) {
	cfg := testRetryConfig(&recorder{}, 5)
	cfg.Sleep = func(context.Context, time.Duration) error { return context.Canceled }
	calls := 0

	_ = Retry(context.Background(), cfg, func(context.Context) error {
		calls++
		return errBoom
	})

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestDefaultRetryableSkipsContextErrorsAndOpenBreaker(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, ErrOpen} {
		calls := 0
		_ = Retry(context.Background(), RetryConfig{Sleep: (&recorder{}).Sleep}, func(context.Context) error {
			calls++
			return err
		})
		if calls != 1 {
			t.Errorf("%v: calls = %d, want 1", err, calls)
		}
	}
}

func TestDefaultRetryableRetriesOtherErrors(t *testing.T) {
	calls := 0
	_ = Retry(context.Background(), RetryConfig{Sleep: (&recorder{}).Sleep}, func(context.Context) error {
		calls++
		return errBoom
	})
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (the default MaxAttempts)", calls)
	}
}

func TestSleepStopsWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSleepWaitsForTheDuration(t *testing.T) {
	if err := sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}
