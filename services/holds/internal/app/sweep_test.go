package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// sweepService is a saga service whose repository the test can also write to
// directly, as a Holds instance that stopped half-way would have left it.
func sweepService(t *testing.T, opts ...Option) (*Service, *memory.Repository, *fakeInventory, *fakeEvents, *clock) {
	t.Helper()
	inv, ev := &fakeInventory{}, &fakeEvents{}
	svc, repo, clk := newTestService(t, append([]Option{WithInventory(inv), WithEvents(ev)}, opts...)...)
	return svc, repo, inv, ev, clk
}

// storeOnly stores a PENDING hold without publishing hold.requested: what is
// left when Holds stops between saving the hold and publishing the event.
func storeOnly(t *testing.T, repo *memory.Repository, id string, at time.Time) domain.Hold {
	t.Helper()
	h, err := domain.NewHold(id, "u-1", "ph-1", "med-1", 2, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(context.Background(), h, 0); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestSweepRecoversAHoldWhosePublishWasKilled(t *testing.T) {
	svc, repo, inv, ev, clk := sweepService(t)
	h := storeOnly(t, repo, "h-lost", clk.now())

	clk.advance(DefaultPendingTimeout() + time.Second)
	res, err := svc.SweepStaleHolds(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got := mustGet(t, svc, h.ID)
	if got.Status != domain.Rejected || got.StatusReason != reasonPendingTimedOut {
		t.Fatalf("hold = %s %q, want REJECTED %q", got.Status, got.StatusReason, reasonPendingTimedOut)
	}
	if got.StockReleasePending {
		t.Error("release mark still set after a successful release")
	}
	if !got.StatusChangedAt.Equal(clk.now()) {
		t.Errorf("status changed at %v, want the sweep time %v", got.StatusChangedAt, clk.now())
	}
	if inv.count("release") != 1 || inv.calls[0].holdID != h.ID {
		t.Errorf("inventory calls = %v, want one release of %s", inv.calls, h.ID)
	}
	if len(ev.rejected) != 1 || ev.rejected[0].ID != h.ID {
		t.Errorf("hold.rejected = %v, want the swept hold", ev.rejected)
	}
	if res != (SweepResult{Rejected: 1, Released: 1}) {
		t.Errorf("result = %+v", res)
	}

	// The patient may hold again: the swept hold no longer counts as open.
	for range DefaultMaxActivePerUser {
		if _, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 1); err != nil {
			t.Fatalf("create after sweep: %v", err)
		}
	}
}

// Inventory reserved the units, but its stock.reserved answer was lost: the
// sweep gives the units back. This is the case where stock stays reserved for ever.
func TestSweepReleasesStockWhenStockReservedWasLost(t *testing.T) {
	svc, _, inv, ev, clk := sweepService(t)
	h, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.requested) != 1 {
		t.Fatalf("hold.requested = %d, want 1", len(ev.requested))
	}
	// ... Inventory reserves; stock.reserved never arrives ...

	clk.advance(DefaultPendingTimeout() + time.Minute)
	if _, err := svc.SweepStaleHolds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, svc, h.ID); got.Status != domain.Rejected {
		t.Fatalf("status = %s, want REJECTED", got.Status)
	}
	if inv.count("release") != 1 {
		t.Fatalf("releases = %d, want 1", inv.count("release"))
	}

	// If stock.reserved turns up after all, the hold stays REJECTED and the
	// reservation is released again (safe to repeat).
	outcome, err := svc.HandleStockReserved(context.Background(), h.ID)
	if err != nil || outcome != OutcomeReleased {
		t.Fatalf("late stock.reserved = %q, %v; want %q", outcome, err, OutcomeReleased)
	}
	if got := mustGet(t, svc, h.ID); got.Status != domain.Rejected {
		t.Errorf("late stock.reserved changed the hold to %s", got.Status)
	}
}

func TestSweepLeavesFreshAndFinishedHoldsAlone(t *testing.T) {
	svc, repo, inv, ev, clk := sweepService(t)
	fresh := storeOnly(t, repo, "h-fresh", clk.now())
	active := activeHold(t, svc)
	clk.advance(DefaultPendingTimeout()) // exactly the timeout: not older than it yet

	res, err := svc.SweepStaleHolds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res != (SweepResult{}) || len(inv.calls) != 0 || len(ev.rejected) != 0 {
		t.Fatalf("result %+v, inventory %v, rejected %v: want nothing done", res, inv.calls, ev.rejected)
	}
	for _, id := range []string{fresh.ID, active.ID} {
		if got := mustGet(t, svc, id); got.Status == domain.Rejected {
			t.Errorf("%s was rejected", id)
		}
	}
}

func TestSweepUsesTheConfiguredTimeout(t *testing.T) {
	inv, ev := &fakeInventory{}, &fakeEvents{}
	repo := memory.NewRepository()
	clk := &clock{t: t0}
	cfg := testConfig()
	cfg.PendingTimeout = time.Minute
	svc := NewService(repo, cfg, clk.now, WithInventory(inv), WithEvents(ev))
	storeOnly(t, repo, "h-1", clk.now())

	clk.advance(time.Minute + time.Second)
	res, err := svc.SweepStaleHolds(context.Background())
	if err != nil || res.Rejected != 1 {
		t.Fatalf("result %+v, %v; want the hold rejected after 1m", res, err)
	}
}

// A reservation that lands while the sweep runs wins: the hold stays ACTIVE and
// its stock is not released.
func TestSweepLosesTheRaceToAReservation(t *testing.T) {
	inv, ev := &fakeInventory{}, &fakeEvents{}
	base := memory.NewRepository()
	hooked := &hookRepo{HoldRepository: base}
	clk := &clock{t: t0}
	svc := NewService(hooked, testConfig(), clk.now,
		WithInventory(inv), WithEvents(ev), WithCodeGenerator(always(testCode)))
	h := storeOnly(t, base, "h-race", clk.now())
	other := storeOnly(t, base, "h-other", clk.now().Add(time.Second)) // swept after h-race
	clk.advance(DefaultPendingTimeout() + 2*time.Second)

	hooked.beforeUpdate = func() {
		if _, err := svc.ReserveHold(context.Background(), h.ID); err != nil {
			t.Fatalf("reserve during sweep: %v", err)
		}
	}
	res, err := svc.SweepStaleHolds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, svc, h.ID); got.Status != domain.Active || got.StockReleasePending {
		t.Fatalf("hold = %s (release pending %v), want ACTIVE", got.Status, got.StockReleasePending)
	}
	// The lost race does not stop the sweep: the next stale hold is still handled.
	if got := mustGet(t, svc, other.ID); got.Status != domain.Rejected {
		t.Fatalf("next stale hold = %s, want REJECTED", got.Status)
	}
	if res != (SweepResult{Rejected: 1, Released: 1}) || len(inv.calls) != 1 || inv.calls[0].holdID != other.ID {
		t.Errorf("result %+v, inventory %v: only %s may be rejected and released", res, inv.calls, other.ID)
	}
	if len(ev.rejected) != 1 || ev.rejected[0].ID != other.ID {
		t.Errorf("hold.rejected = %v, want only %s", ev.rejected, other.ID)
	}
}

// One hold's trouble never stops the sweep: every later hold in the batch is
// still handled. Each case makes the first hold fail at a different step.
func TestSweepGoesOnAfterOneHoldFails(t *testing.T) {
	t.Run("release fails inside the retry window", func(t *testing.T) {
		svc, repo, inv, _, clk := sweepService(t)
		storeOnly(t, repo, "h-1", clk.now())
		storeOnly(t, repo, "h-2", clk.now().Add(time.Second))
		clk.advance(DefaultPendingTimeout() + 2*time.Second)
		inv.failFor = map[string]error{"h-1": domain.ErrInventoryUnavailable}

		res, _ := svc.SweepStaleHolds(context.Background())
		if res != (SweepResult{Rejected: 2, Released: 1, ReleaseFailed: 1}) {
			t.Fatalf("result = %+v", res)
		}
		if mustGet(t, svc, "h-2").StockReleasePending {
			t.Error("h-2 was not released after h-1 failed")
		}
	})

	t.Run("release gives up", func(t *testing.T) {
		svc, repo, inv, _, clk := sweepService(t)
		storeOnly(t, repo, "h-old", clk.now())
		clk.advance(DefaultPendingTimeout() + time.Second)
		inv.err = errBoom
		_, _ = svc.SweepStaleHolds(context.Background()) // h-old: rejected, release failed

		clk.advance(releaseRetryWindow)
		storeOnly(t, repo, "h-new", clk.now().Add(-DefaultPendingTimeout()-time.Second))
		inv.err = nil
		inv.failFor = map[string]error{"h-old": errBoom}

		res, _ := svc.SweepStaleHolds(context.Background())
		if res != (SweepResult{Rejected: 1, Released: 1, GaveUp: 1}) {
			t.Fatalf("result = %+v, want h-old given up and h-new released", res)
		}
	})

	t.Run("clearing the release mark fails", func(t *testing.T) {
		repo := updateFails{memory.NewRepository(), func(h domain.Hold, from domain.Status) bool {
			return h.ID == "h-1" && from == domain.Rejected
		}}
		clk := &clock{t: t0}
		svc := NewService(repo, testConfig(), clk.now)
		storeOnly(t, repo.Repository, "h-1", clk.now())
		storeOnly(t, repo.Repository, "h-2", clk.now().Add(time.Second))
		clk.advance(DefaultPendingTimeout() + 2*time.Second)

		res, err := svc.SweepStaleHolds(context.Background())
		if !errors.Is(err, errBoom) || res != (SweepResult{Rejected: 2, Released: 1}) {
			t.Fatalf("result = %+v, %v; want h-2 released", res, err)
		}
	})
}

func TestSweepRetriesAFailedReleaseOnTheNextSweep(t *testing.T) {
	svc, repo, inv, _, clk := sweepService(t)
	h := storeOnly(t, repo, "h-1", clk.now())
	clk.advance(DefaultPendingTimeout() + time.Second)

	inv.err = domain.ErrInventoryUnavailable
	res, err := svc.SweepStaleHolds(context.Background())
	if !errors.Is(err, domain.ErrInventoryUnavailable) || res != (SweepResult{Rejected: 1, ReleaseFailed: 1}) {
		t.Fatalf("first sweep = %+v, %v", res, err)
	}
	if got := mustGet(t, svc, h.ID); got.Status != domain.Rejected || !got.StockReleasePending {
		t.Fatalf("hold = %s (release pending %v), want REJECTED and still to release", got.Status, got.StockReleasePending)
	}

	inv.err = nil
	clk.advance(time.Minute)
	res, err = svc.SweepStaleHolds(context.Background())
	if err != nil || res != (SweepResult{Released: 1}) {
		t.Fatalf("second sweep = %+v, %v; want the release done", res, err)
	}
	if mustGet(t, svc, h.ID).StockReleasePending {
		t.Error("release mark still set")
	}

	// Nothing is left: a third sweep does nothing.
	res, err = svc.SweepStaleHolds(context.Background())
	if err != nil || res != (SweepResult{}) || inv.count("release") != 2 {
		t.Fatalf("third sweep = %+v, %v, releases %d", res, err, inv.count("release"))
	}
}

func TestSweepGivesUpAfterTheRetryWindow(t *testing.T) {
	svc, repo, inv, _, clk := sweepService(t)
	h := storeOnly(t, repo, "h-1", clk.now())
	clk.advance(DefaultPendingTimeout() + time.Second)
	inv.err = errBoom // for example "no such reservation": hold.requested never reached Inventory

	if _, err := svc.SweepStaleHolds(context.Background()); err == nil {
		t.Fatal("want the failed release reported")
	}
	clk.advance(releaseRetryWindow - time.Second)
	if res, _ := svc.SweepStaleHolds(context.Background()); res.ReleaseFailed != 1 || res.GaveUp != 0 {
		t.Fatalf("inside the window: %+v, want still retrying", res)
	}

	clk.advance(time.Second)
	res, err := svc.SweepStaleHolds(context.Background())
	if res != (SweepResult{GaveUp: 1}) || err == nil || !strings.Contains(err.Error(), "gave up") {
		t.Fatalf("after the window: %+v, %v; want gave up", res, err)
	}
	if got := mustGet(t, svc, h.ID); got.StockReleasePending || got.Status != domain.Rejected {
		t.Fatalf("hold = %s (release pending %v), want REJECTED without the mark", got.Status, got.StockReleasePending)
	}
}

func TestSweepStillReleasesWhenHoldRejectedCannotBePublished(t *testing.T) {
	svc, repo, inv, ev, clk := sweepService(t)
	storeOnly(t, repo, "h-1", clk.now())
	clk.advance(DefaultPendingTimeout() + time.Second)
	ev.errRejected = errBoom

	res, err := svc.SweepStaleHolds(context.Background())
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the publish failure", err)
	}
	if res != (SweepResult{Rejected: 1, Released: 1}) || inv.count("release") != 1 {
		t.Fatalf("result %+v, releases %d", res, inv.count("release"))
	}
}

func TestSweepReportsRepositoryErrors(t *testing.T) {
	svc := NewService(errRepo{}, testConfig(), func() time.Time { return t0 })
	if _, err := svc.SweepStaleHolds(context.Background()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the listing error", err)
	}
}

// failingReleaseList lists stale holds normally but cannot list holds to release.
type failingReleaseList struct{ *memory.Repository }

func (failingReleaseList) ListReleasePending(context.Context, int) ([]domain.Hold, error) {
	return nil, errBoom
}

func TestSweepReportsAFailedReleaseListing(t *testing.T) {
	repo := failingReleaseList{memory.NewRepository()}
	clk := &clock{t: t0}
	svc := NewService(repo, testConfig(), clk.now)
	storeOnly(t, repo.Repository, "h-1", clk.now())
	clk.advance(DefaultPendingTimeout() + time.Second)

	res, err := svc.SweepStaleHolds(context.Background())
	if !errors.Is(err, errBoom) || res.Rejected != 1 {
		t.Fatalf("result %+v, %v; want the hold rejected and the listing error", res, err)
	}
}

// updateFails lets every Update through except those that match fail.
type updateFails struct {
	*memory.Repository
	fail func(h domain.Hold, from domain.Status) bool
}

func (r updateFails) Update(ctx context.Context, h domain.Hold, from domain.Status) error {
	if r.fail(h, from) {
		return errBoom
	}
	return r.Repository.Update(ctx, h, from)
}

func TestSweepReportsUpdateErrors(t *testing.T) {
	t.Run("rejecting", func(t *testing.T) {
		repo := updateFails{memory.NewRepository(), func(h domain.Hold, _ domain.Status) bool { return h.Status == domain.Rejected }}
		clk := &clock{t: t0}
		svc := NewService(repo, testConfig(), clk.now)
		storeOnly(t, repo.Repository, "h-1", clk.now())
		clk.advance(DefaultPendingTimeout() + time.Second)

		res, err := svc.SweepStaleHolds(context.Background())
		if !errors.Is(err, errBoom) || res.Rejected != 0 {
			t.Fatalf("result %+v, %v", res, err)
		}
	})
	t.Run("clearing the release mark", func(t *testing.T) {
		repo := updateFails{memory.NewRepository(), func(h domain.Hold, from domain.Status) bool {
			return from == domain.Rejected && !h.StockReleasePending
		}}
		clk := &clock{t: t0}
		svc := NewService(repo, testConfig(), clk.now)
		storeOnly(t, repo.Repository, "h-1", clk.now())
		clk.advance(DefaultPendingTimeout() + time.Second)

		res, err := svc.SweepStaleHolds(context.Background())
		if !errors.Is(err, errBoom) || res != (SweepResult{Rejected: 1}) {
			t.Fatalf("result %+v, %v", res, err)
		}
	})
}

// A hold that is already REJECTED cannot be rejected by the sweep; the sweep
// reports it and goes on with the rest.
type staleLister struct {
	*memory.Repository
	extra domain.Hold
}

func (r staleLister) ListStalePending(ctx context.Context, before time.Time, limit int) ([]domain.Hold, error) {
	holds, err := r.Repository.ListStalePending(ctx, before, limit)
	return append([]domain.Hold{r.extra}, holds...), err
}

func TestSweepSkipsAHoldItCannotReject(t *testing.T) {
	inner := memory.NewRepository()
	notPending := domain.Hold{ID: "h-odd", Status: domain.Collected}
	repo := staleLister{inner, notPending}
	clk := &clock{t: t0}
	svc := NewService(repo, testConfig(), clk.now)
	storeOnly(t, inner, "h-1", clk.now())
	clk.advance(DefaultPendingTimeout() + time.Second)

	res, err := svc.SweepStaleHolds(context.Background())
	if !errors.Is(err, domain.ErrInvalidTransition) || res.Rejected != 1 {
		t.Fatalf("result %+v, %v; want h-1 rejected and h-odd reported", res, err)
	}
}
