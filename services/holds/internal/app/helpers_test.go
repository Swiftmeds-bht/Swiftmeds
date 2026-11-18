package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

var t0 = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

const testCode = "ABC234"

var errBoom = errors.New("boom")

// clock is a clock the test moves by hand.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

// sequence returns a generator that gives the values one after another and
// then fails, so a test notices if the service asks for more than expected.
func sequence(values ...string) func() (string, error) {
	i := 0
	return func() (string, error) {
		if i >= len(values) {
			return "", errors.New("generator ran out of values")
		}
		v := values[i]
		i++
		return v, nil
	}
}

// always returns a generator that gives the same value every time.
func always(value string) func() (string, error) {
	return func() (string, error) { return value, nil }
}

func testConfig() Config {
	return Config{MaxActivePerUser: 3, HoldDuration: 2 * time.Hour}
}

// newTestService builds a Service on an in-memory repository, a clock stuck at t0,
// hold IDs h-1, h-2, ... and the pickup code ABC234 (unless an option says otherwise).
func newTestService(t *testing.T, opts ...Option) (*Service, *memory.Repository, *clock) {
	t.Helper()
	repo := memory.NewRepository()
	clk := &clock{t: t0}
	n := 0
	ids := func() (string, error) {
		n++
		return "h-" + string(rune('0'+n)), nil
	}
	all := append([]Option{WithIDGenerator(ids), WithCodeGenerator(always(testCode))}, opts...)
	return NewService(repo, testConfig(), clk.now, all...), repo, clk
}

// activeHold creates a hold for patient u-1 at pharmacy ph-1 and reserves it.
func activeHold(t *testing.T, svc *Service) domain.Hold {
	t.Helper()
	ctx := context.Background()
	h, err := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	active, err := svc.ReserveHold(ctx, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	return active
}

func mustGet(t *testing.T, svc *Service, id string) domain.Hold {
	t.Helper()
	h, err := svc.GetHold(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// errRepo fails every call, to test how the service reports repository errors.
type errRepo struct{}

func (errRepo) Create(context.Context, domain.Hold, int) error { return errBoom }
func (errRepo) Get(context.Context, string) (domain.Hold, error) {
	return domain.Hold{}, errBoom
}
func (errRepo) Update(context.Context, domain.Hold, domain.Status) error { return errBoom }
func (errRepo) List(context.Context, domain.ListFilter) ([]domain.Hold, error) {
	return nil, errBoom
}
func (errRepo) RecordWrongPickupCode(context.Context, string) (int, error) { return 0, errBoom }
func (errRepo) ListStalePending(context.Context, time.Time, int) ([]domain.Hold, error) {
	return nil, errBoom
}
func (errRepo) ListReleasePending(context.Context, int) ([]domain.Hold, error) {
	return nil, errBoom
}

var _ ports.HoldRepository = errRepo{}

// hookRepo runs a function once, just before the next Update. A test uses it to
// make another caller change the hold between the service's read and its write.
type hookRepo struct {
	ports.HoldRepository
	beforeUpdate func()
	lastList     domain.ListFilter
}

func (r *hookRepo) Update(ctx context.Context, h domain.Hold, from domain.Status) error {
	if hook := r.beforeUpdate; hook != nil {
		r.beforeUpdate = nil
		hook()
	}
	return r.HoldRepository.Update(ctx, h, from)
}

func (r *hookRepo) List(ctx context.Context, f domain.ListFilter) ([]domain.Hold, error) {
	r.lastList = f
	return r.HoldRepository.List(ctx, f)
}
