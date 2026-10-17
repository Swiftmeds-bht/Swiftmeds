package resilience

import (
	"errors"
	"sync"
	"time"
)

// ErrOpen is returned when the breaker is open and the call was not tried.
var ErrOpen = errors.New("resilience: circuit breaker is open")

// State is the state of a circuit breaker.
type State int

const (
	// Closed lets every call through and counts failures.
	Closed State = iota
	// Open rejects every call until the cooldown has passed.
	Open
	// HalfOpen lets one trial call through to test the service.
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	default:
		return "half-open"
	}
}

// BreakerConfig holds the breaker settings. Zero fields use the defaults.
type BreakerConfig struct {
	// FailureThreshold is how many failures in a row open the breaker. Default 5.
	FailureThreshold int
	// Cooldown is how long the breaker stays open before a trial call. Default 10s.
	Cooldown time.Duration
	// IsFailure says which errors count as failures. Default: every error.
	IsFailure func(error) bool
	// Now is the clock. Default time.Now. Tests replace it.
	Now func() time.Time
}

// Breaker is a circuit breaker. It is safe for concurrent use.
type Breaker struct {
	cfg BreakerConfig

	mu       sync.Mutex
	state    State
	failures int
	openedAt time.Time
	probing  bool // a trial call is running in the half-open state
}

// NewBreaker returns a closed breaker.
func NewBreaker(cfg BreakerConfig) *Breaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 10 * time.Second
	}
	if cfg.IsFailure == nil {
		cfg.IsFailure = func(error) bool { return true }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Breaker{cfg: cfg}
}

// State returns the current state.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()
	return b.state
}

// Execute runs fn unless the breaker is open, and records the result.
// It returns ErrOpen without calling fn when the call is rejected.
func (b *Breaker) Execute(fn func() error) error {
	probe, err := b.allow()
	if err != nil {
		return err
	}
	err = fn()
	b.record(err, probe)
	return err
}

// refresh moves Open to HalfOpen once the cooldown has passed. Hold the lock.
func (b *Breaker) refresh() {
	if b.state == Open && b.cfg.Now().Sub(b.openedAt) >= b.cfg.Cooldown {
		b.state = HalfOpen
		b.probing = false
	}
}

// allow decides whether a call may run. probe is true for the half-open trial call.
func (b *Breaker) allow() (probe bool, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refresh()

	switch b.state {
	case Open:
		return false, ErrOpen
	case HalfOpen:
		if b.probing {
			return false, ErrOpen // only one trial call at a time
		}
		b.probing = true
		return true, nil
	}
	return false, nil
}

func (b *Breaker) record(err error, probe bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	failed := err != nil && b.cfg.IsFailure(err)

	switch {
	case probe && b.state == HalfOpen:
		b.probing = false
		if failed {
			b.trip()
		} else {
			b.state = Closed
			b.failures = 0
		}
	case !probe && b.state == Closed:
		if !failed {
			b.failures = 0
			return
		}
		b.failures++
		if b.failures >= b.cfg.FailureThreshold {
			b.trip()
		}
	}
}

// trip opens the breaker and starts the cooldown. Hold the lock.
func (b *Breaker) trip() {
	b.state = Open
	b.openedAt = b.cfg.Now()
	b.failures = 0
}
