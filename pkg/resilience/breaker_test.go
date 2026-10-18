package resilience

import (
	"errors"
	"testing"
	"time"
)

var errBoom = errors.New("boom")

// fakeClock lets tests move time by hand.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

const testCooldown = 10 * time.Second

func newTestBreaker(threshold int) (*Breaker, *fakeClock) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	b := NewBreaker(BreakerConfig{
		FailureThreshold: threshold,
		Cooldown:         testCooldown,
		Now:              clock.Now,
	})
	return b, clock
}

func fail() error { return errBoom }
func ok() error   { return nil }

// openBreaker makes threshold failing calls so the breaker is open.
func openBreaker(t *testing.T, b *Breaker, threshold int) {
	t.Helper()
	for range threshold {
		_ = b.Execute(fail)
	}
	if b.State() != Open {
		t.Fatalf("state = %v, want open", b.State())
	}
}

func TestBreakerStartsClosedAndPassesCalls(t *testing.T) {
	b, _ := newTestBreaker(3)

	if b.State() != Closed {
		t.Fatalf("state = %v, want closed", b.State())
	}
	if err := b.Execute(ok); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if err := b.Execute(fail); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the error from the call", err)
	}
}

func TestBreakerClosedToOpenAfterThresholdFailures(t *testing.T) {
	b, _ := newTestBreaker(3)

	_ = b.Execute(fail)
	_ = b.Execute(fail)
	if b.State() != Closed {
		t.Fatalf("after 2 failures state = %v, want closed", b.State())
	}
	_ = b.Execute(fail)
	if b.State() != Open {
		t.Fatalf("after 3 failures state = %v, want open", b.State())
	}
}

func TestBreakerOpenRejectsCallsWithoutRunningThem(t *testing.T) {
	b, _ := newTestBreaker(2)
	openBreaker(t, b, 2)

	called := false
	err := b.Execute(func() error { called = true; return nil })

	if !errors.Is(err, ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen", err)
	}
	if called {
		t.Error("call ran while the breaker was open")
	}
}

func TestBreakerSuccessResetsTheFailureCount(t *testing.T) {
	b, _ := newTestBreaker(3)

	_ = b.Execute(fail)
	_ = b.Execute(fail)
	_ = b.Execute(ok)
	_ = b.Execute(fail)
	_ = b.Execute(fail)

	if b.State() != Closed {
		t.Fatalf("state = %v, want closed (failures were not in a row)", b.State())
	}
}

func TestBreakerOpenToHalfOpenAfterCooldown(t *testing.T) {
	b, clock := newTestBreaker(2)
	openBreaker(t, b, 2)

	clock.Advance(testCooldown - time.Nanosecond)
	if b.State() != Open {
		t.Fatalf("just before cooldown state = %v, want open", b.State())
	}
	clock.Advance(time.Nanosecond)
	if b.State() != HalfOpen {
		t.Fatalf("after cooldown state = %v, want half-open", b.State())
	}
}

func TestBreakerHalfOpenToClosedWhenTrialCallSucceeds(t *testing.T) {
	b, clock := newTestBreaker(2)
	openBreaker(t, b, 2)
	clock.Advance(testCooldown)

	if err := b.Execute(ok); err != nil {
		t.Fatalf("trial call err = %v, want nil", err)
	}
	if b.State() != Closed {
		t.Fatalf("state = %v, want closed", b.State())
	}

	// The failure count started again from zero.
	_ = b.Execute(fail)
	if b.State() != Closed {
		t.Fatalf("one failure after closing: state = %v, want closed", b.State())
	}
}

func TestBreakerHalfOpenToOpenWhenTrialCallFails(t *testing.T) {
	b, clock := newTestBreaker(2)
	openBreaker(t, b, 2)
	clock.Advance(testCooldown)

	if err := b.Execute(fail); !errors.Is(err, errBoom) {
		t.Fatalf("trial call err = %v, want the error from the call", err)
	}
	if b.State() != Open {
		t.Fatalf("state = %v, want open", b.State())
	}

	// The cooldown starts again from the failed trial call.
	clock.Advance(testCooldown - time.Nanosecond)
	if err := b.Execute(ok); !errors.Is(err, ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen before the new cooldown ends", err)
	}
	clock.Advance(time.Nanosecond)
	if b.State() != HalfOpen {
		t.Fatalf("state = %v, want half-open", b.State())
	}
}

func TestBreakerHalfOpenAllowsOnlyOneTrialCall(t *testing.T) {
	b, clock := newTestBreaker(2)
	openBreaker(t, b, 2)
	clock.Advance(testCooldown)

	var second error
	_ = b.Execute(func() error {
		second = b.Execute(ok) // a second call while the trial call is running
		return nil
	})

	if !errors.Is(second, ErrOpen) {
		t.Fatalf("second call err = %v, want ErrOpen", second)
	}
}

func TestBreakerIgnoresErrorsThatAreNotFailures(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	errNotFound := errors.New("not found")
	b := NewBreaker(BreakerConfig{
		FailureThreshold: 2,
		Now:              clock.Now,
		IsFailure:        func(err error) bool { return !errors.Is(err, errNotFound) },
	})

	for range 10 {
		_ = b.Execute(func() error { return errNotFound })
	}

	if b.State() != Closed {
		t.Fatalf("state = %v, want closed", b.State())
	}
}

func TestBreakerDefaults(t *testing.T) {
	b := NewBreaker(BreakerConfig{})

	for range 4 {
		_ = b.Execute(fail)
	}
	if b.State() != Closed {
		t.Fatalf("after 4 failures state = %v, want closed (default threshold is 5)", b.State())
	}
	_ = b.Execute(fail)
	if b.State() != Open {
		t.Fatalf("after 5 failures state = %v, want open", b.State())
	}
}

func TestStateString(t *testing.T) {
	for state, want := range map[State]string{Closed: "closed", Open: "open", HalfOpen: "half-open"} {
		if got := state.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", state, got, want)
		}
	}
}
