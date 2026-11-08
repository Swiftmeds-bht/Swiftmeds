package app

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/ports"
)

var t0 = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

// clock is a clock the test moves by hand. The service and the fake Holds
// service share it, so they agree on "now".
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

type fakeHold struct {
	status    domain.HoldStatus
	expiresAt time.Time
}

// fakeHolds behaves like the real Holds service: ExpireHold does nothing to a
// hold that is not ACTIVE, refuses a hold whose time has not come, and
// otherwise expires it. It counts the calls per hold.
type fakeHolds struct {
	mu     sync.Mutex
	now    func() time.Time
	holds  map[string]*fakeHold
	calls  map[string]int
	errFor map[string]error
	// afterCall runs at the end of every ExpireHold call.
	afterCall func()
}

func (f *fakeHolds) add(id string, status domain.HoldStatus, expiresAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.holds[id] = &fakeHold{status: status, expiresAt: expiresAt}
}

func (f *fakeHolds) statusOf(id string) domain.HoldStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.holds[id].status
}

func (f *fakeHolds) callCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[id]
}

func (f *fakeHolds) totalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, n := range f.calls {
		total += n
	}
	return total
}

func (f *fakeHolds) ExpireHold(_ context.Context, id string) (domain.HoldStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[id]++
	if f.afterCall != nil {
		defer f.afterCall()
	}
	if err, ok := f.errFor[id]; ok {
		return "", err
	}
	h, ok := f.holds[id]
	if !ok {
		return "", domain.ErrHoldNotFound
	}
	if h.status != domain.HoldActive {
		return h.status, nil
	}
	if f.now().Before(h.expiresAt) {
		return "", domain.ErrNotYetExpired
	}
	h.status = domain.HoldExpired
	return h.status, nil
}

var _ ports.HoldExpirer = (*fakeHolds)(nil)

type env struct {
	svc   *Service
	repo  *memory.Repository
	holds *fakeHolds
	clk   *clock
}

func newEnv(t *testing.T, cfg Config) *env {
	t.Helper()
	clk := &clock{t: t0}
	repo := memory.NewRepository()
	holds := &fakeHolds{
		now:    clk.now,
		holds:  map[string]*fakeHold{},
		calls:  map[string]int{},
		errFor: map[string]error{},
	}
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &env{svc: NewService(repo, holds, cfg, clk.now, discard), repo: repo, holds: holds, clk: clk}
}

// confirm makes a hold ACTIVE in the fake Holds service and records its
// deadline in Expiry, the way the hold.confirmed event does.
func (e *env) confirm(t *testing.T, id string, expiresAt time.Time) {
	t.Helper()
	e.holds.add(id, domain.HoldActive, expiresAt)
	if err := e.svc.RecordDeadline(context.Background(), id, expiresAt); err != nil {
		t.Fatal(err)
	}
}

// sweep runs a sweep and fails the test on an error.
func (e *env) sweep(t *testing.T, asOf time.Time) SweepResult {
	t.Helper()
	res, err := e.svc.RunSweep(context.Background(), asOf)
	if err != nil {
		t.Fatalf("RunSweep: %v", err)
	}
	return res
}

// failingRepo is a repository whose chosen calls fail.
type failingRepo struct {
	ports.DeadlineRepository
	saveErr     error
	listErr     error
	completeErr error
}

func (r failingRepo) Save(ctx context.Context, d domain.Deadline) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	return r.DeadlineRepository.Save(ctx, d)
}

func (r failingRepo) ListDue(ctx context.Context, asOf time.Time, after *domain.Deadline, limit int) ([]domain.Deadline, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.DeadlineRepository.ListDue(ctx, asOf, after, limit)
}

func (r failingRepo) Complete(ctx context.Context, id string, o domain.Outcome, at time.Time) (bool, error) {
	if r.completeErr != nil {
		return false, r.completeErr
	}
	return r.DeadlineRepository.Complete(ctx, id, o, at)
}
