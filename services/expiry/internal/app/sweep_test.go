package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
)

func TestSweepExpiresTheHoldsThatAreDue(t *testing.T) {
	e := newEnv(t, Config{})
	e.confirm(t, "h-1", t0.Add(-2*time.Minute))
	e.confirm(t, "h-2", t0.Add(-time.Minute))
	e.confirm(t, "h-later", t0.Add(time.Hour))

	res := e.sweep(t, t0)

	if want := []string{"h-1", "h-2"}; !slices.Equal(res.Expired, want) {
		t.Errorf("expired = %v, want %v", res.Expired, want)
	}
	if res.Deferred != 0 || res.Failed != 0 {
		t.Errorf("deferred = %d, failed = %d, want 0 and 0", res.Deferred, res.Failed)
	}
	if got := e.holds.statusOf("h-later"); got != domain.HoldActive {
		t.Errorf("h-later is %s, want it left ACTIVE", got)
	}
	if got := e.holds.callCount("h-later"); got != 0 {
		t.Errorf("Holds was asked about h-later %d times, want 0", got)
	}
}

// The ticket: "Running it twice in a row expires each hold once (idempotent)."
func TestRunningTheSweepTwiceExpiresEachHoldOnce(t *testing.T) {
	e := newEnv(t, Config{})
	e.confirm(t, "h-1", t0.Add(-time.Minute))
	e.confirm(t, "h-2", t0.Add(-time.Second))

	first := e.sweep(t, t0)
	second := e.sweep(t, t0)

	if want := []string{"h-1", "h-2"}; !slices.Equal(first.Expired, want) {
		t.Errorf("first sweep expired %v, want %v", first.Expired, want)
	}
	if len(second.Expired) != 0 {
		t.Errorf("second sweep expired %v, want nothing", second.Expired)
	}
	for _, id := range []string{"h-1", "h-2"} {
		if got := e.holds.callCount(id); got != 1 {
			t.Errorf("Holds.ExpireHold(%s) was called %d times, want exactly 1", id, got)
		}
		if got := e.holds.statusOf(id); got != domain.HoldExpired {
			t.Errorf("%s is %s, want EXPIRED", id, got)
		}
	}
}

// The ticket: "Boundary test: a hold expiring exactly now."
func TestAHoldExpiringExactlyNowIsExpired(t *testing.T) {
	e := newEnv(t, Config{})
	e.confirm(t, "h-before", t0.Add(-time.Nanosecond))
	e.confirm(t, "h-now", t0)
	e.confirm(t, "h-after", t0.Add(time.Nanosecond))

	res := e.sweep(t, t0)

	if want := []string{"h-before", "h-now"}; !slices.Equal(res.Expired, want) {
		t.Fatalf("expired = %v, want %v: a hold expiring exactly now is due, one a nanosecond later is not", res.Expired, want)
	}
	if got := e.holds.statusOf("h-now"); got != domain.HoldExpired {
		t.Errorf("h-now is %s, want EXPIRED", got)
	}
	if got := e.holds.statusOf("h-after"); got != domain.HoldActive {
		t.Errorf("h-after is %s, want it left ACTIVE", got)
	}
}

// docs/test-plan.md TC-011: just before expiry stays ACTIVE; at and after, EXPIRED.
func TestHoldBoundaryWithAnInjectedClock(t *testing.T) {
	cases := []struct {
		name string
		asOf time.Time
		want domain.HoldStatus
	}{
		{"one second before expiry", t0.Add(-time.Second), domain.HoldActive},
		{"at expiry", t0, domain.HoldExpired},
		{"one second after expiry", t0.Add(time.Second), domain.HoldExpired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, Config{})
			e.clk.t = c.asOf // Holds' clock agrees with as_of
			e.confirm(t, "h-1", t0)

			e.sweep(t, c.asOf)

			if got := e.holds.statusOf("h-1"); got != c.want {
				t.Errorf("status = %s, want %s", got, c.want)
			}
		})
	}
}

func TestSweepWithoutAsOfUsesTheClock(t *testing.T) {
	e := newEnv(t, Config{})
	e.confirm(t, "h-1", t0.Add(-time.Minute))

	res := e.sweep(t, time.Time{})

	if want := []string{"h-1"}; !slices.Equal(res.Expired, want) {
		t.Errorf("expired = %v, want %v", res.Expired, want)
	}
}

func TestSweepWithNothingDue(t *testing.T) {
	e := newEnv(t, Config{})

	res := e.sweep(t, t0)

	if len(res.Expired) != 0 || res.Deferred != 0 || res.Failed != 0 || e.holds.totalCalls() != 0 {
		t.Errorf("got %+v and %d calls to Holds, want an empty sweep", res, e.holds.totalCalls())
	}
}

func TestAHoldThatWasAlreadyCollectedIsSettledButNotReported(t *testing.T) {
	e := newEnv(t, Config{})
	e.confirm(t, "h-1", t0.Add(-time.Minute))
	e.holds.holds["h-1"].status = domain.HoldCollected

	first := e.sweep(t, t0)
	second := e.sweep(t, t0)

	if len(first.Expired) != 0 || len(second.Expired) != 0 {
		t.Errorf("reported %v and %v, want nothing: the hold was collected, not expired", first.Expired, second.Expired)
	}
	if got := e.holds.callCount("h-1"); got != 1 {
		t.Errorf("Holds was asked %d times, want 1 (the deadline is settled after the first sweep)", got)
	}
}

func TestADeadlineForAnUnknownHoldIsSettledOnce(t *testing.T) {
	e := newEnv(t, Config{})
	if err := e.svc.RecordDeadline(context.Background(), "ghost", t0.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	first := e.sweep(t, t0)
	e.sweep(t, t0)

	if len(first.Expired) != 0 || first.Failed != 0 {
		t.Errorf("got %+v, want nothing reported and no failure", first)
	}
	if got := e.holds.callCount("ghost"); got != 1 {
		t.Errorf("Holds was asked %d times, want 1 (not found settles the deadline)", got)
	}
}

func TestAHoldThatHoldsDoesNotConsiderExpiredYetWaitsForTheNextSweep(t *testing.T) {
	e := newEnv(t, Config{})
	e.confirm(t, "h-1", t0)
	e.clk.advance(-3 * time.Second) // Holds' clock is a few seconds behind the caller's as_of

	first := e.sweep(t, t0)

	if len(first.Expired) != 0 || first.Deferred != 1 {
		t.Fatalf("first sweep = %+v, want one deferred hold and nothing expired", first)
	}
	e.clk.advance(3 * time.Second)
	second := e.sweep(t, t0)
	if want := []string{"h-1"}; !slices.Equal(second.Expired, want) {
		t.Errorf("second sweep expired %v, want %v", second.Expired, want)
	}
}

func TestAHoldThatIsStillActiveAfterExpireHoldIsDeferred(t *testing.T) {
	e := newEnv(t, Config{})
	e.confirm(t, "h-1", t0.Add(-time.Minute))
	// Holds answers without an error but reports a hold that did not change.
	e.svc.holds = &stubHolds{status: domain.HoldActive}

	res := e.sweep(t, t0)

	if res.Deferred != 1 || len(res.Expired) != 0 {
		t.Errorf("got %+v, want one deferred hold", res)
	}
}

func TestSweepStopsWhenHoldsIsUnavailableAndTheNextSweepFinishesTheJob(t *testing.T) {
	e := newEnv(t, Config{})
	e.confirm(t, "h-1", t0.Add(-3*time.Minute))
	e.confirm(t, "h-2", t0.Add(-2*time.Minute))
	e.confirm(t, "h-3", t0.Add(-time.Minute))
	e.holds.errFor["h-1"] = domain.ErrHoldsUnavailable

	res, err := e.svc.RunSweep(context.Background(), t0)

	if !errors.Is(err, domain.ErrHoldsUnavailable) {
		t.Fatalf("err = %v, want ErrHoldsUnavailable", err)
	}
	if res.Failed != 1 || len(res.Expired) != 0 {
		t.Errorf("result = %+v, want one failure and nothing expired", res)
	}
	if e.holds.callCount("h-2") != 0 || e.holds.callCount("h-3") != 0 {
		t.Error("the sweep kept calling Holds after it was found to be down")
	}

	delete(e.holds.errFor, "h-1")
	next := e.sweep(t, t0)
	if want := []string{"h-1", "h-2", "h-3"}; !slices.Equal(next.Expired, want) {
		t.Errorf("next sweep expired %v, want %v", next.Expired, want)
	}
}

func TestOneBrokenHoldDoesNotBlockTheOthers(t *testing.T) {
	e := newEnv(t, Config{BatchSize: 2})
	e.confirm(t, "h-1", t0.Add(-4*time.Minute))
	e.confirm(t, "h-2", t0.Add(-3*time.Minute))
	e.confirm(t, "h-3", t0.Add(-2*time.Minute))
	e.confirm(t, "h-4", t0.Add(-time.Minute))
	e.holds.errFor["h-1"] = errors.New("boom")

	res := e.sweep(t, t0)

	if want := []string{"h-2", "h-3", "h-4"}; !slices.Equal(res.Expired, want) {
		t.Errorf("expired = %v, want %v", res.Expired, want)
	}
	if res.Failed != 1 {
		t.Errorf("failed = %d, want 1", res.Failed)
	}

	delete(e.holds.errFor, "h-1")
	retry := e.sweep(t, t0)
	if want := []string{"h-1"}; !slices.Equal(retry.Expired, want) {
		t.Errorf("the broken hold was expired by the next sweep as %v, want %v", retry.Expired, want)
	}
}

func TestSweepWorksThroughMoreDueHoldsThanOneBatch(t *testing.T) {
	e := newEnv(t, Config{BatchSize: 3})
	var want []string
	for i := range 10 {
		id := fmt.Sprintf("h-%02d", i)
		e.confirm(t, id, t0.Add(-time.Duration(20-i)*time.Minute))
		want = append(want, id)
	}

	res := e.sweep(t, t0)

	if !slices.Equal(res.Expired, want) {
		t.Errorf("expired = %v, want all ten in deadline order %v", res.Expired, want)
	}
}

func TestSweepStopsWhenTheContextEnds(t *testing.T) {
	e := newEnv(t, Config{})
	for i := range 5 {
		e.confirm(t, fmt.Sprintf("h-%d", i), t0.Add(-time.Duration(10-i)*time.Minute))
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.holds.afterCall = cancel

	res, err := e.svc.RunSweep(ctx, t0)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if want := []string{"h-0"}; !slices.Equal(res.Expired, want) {
		t.Errorf("expired = %v, want only the hold done before the cancel: %v", res.Expired, want)
	}

	e.holds.afterCall = nil
	rest := e.sweep(t, t0)
	if want := []string{"h-1", "h-2", "h-3", "h-4"}; !slices.Equal(rest.Expired, want) {
		t.Errorf("next sweep expired %v, want the rest %v", rest.Expired, want)
	}
}

func TestSweepWithAnAlreadyEndedContextDoesNothing(t *testing.T) {
	e := newEnv(t, Config{})
	e.confirm(t, "h-1", t0.Add(-time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.svc.RunSweep(ctx, t0)

	if !errors.Is(err, context.Canceled) || e.holds.totalCalls() != 0 {
		t.Errorf("err = %v with %d calls to Holds, want context.Canceled and no calls", err, e.holds.totalCalls())
	}
}

// Two sweeps at the same moment (an overlapping schedule) must report each hold once.
func TestTwoSweepsAtTheSameTimeReportEachHoldOnce(t *testing.T) {
	e := newEnv(t, Config{BatchSize: 4})
	const holds = 25
	for i := range holds {
		e.confirm(t, fmt.Sprintf("h-%02d", i), t0.Add(-time.Duration(holds-i)*time.Minute))
	}

	var wg sync.WaitGroup
	reports := make(chan []string, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := e.svc.RunSweep(context.Background(), t0)
			if err != nil {
				t.Errorf("RunSweep: %v", err)
			}
			reports <- res.Expired
		}()
	}
	wg.Wait()
	close(reports)

	seen := map[string]int{}
	for ids := range reports {
		for _, id := range ids {
			seen[id]++
		}
	}
	if len(seen) != holds {
		t.Errorf("%d holds were reported, want %d", len(seen), holds)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("%s was reported %d times, want once", id, n)
		}
	}
}

func TestSweepReportsARepositoryErrorWhenListing(t *testing.T) {
	e := newEnv(t, Config{})
	e.svc.repo = failingRepo{DeadlineRepository: e.repo, listErr: errBoom}

	_, err := e.svc.RunSweep(context.Background(), t0)

	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestSweepStopsWhenADeadlineCannotBeSettled(t *testing.T) {
	e := newEnv(t, Config{})
	e.confirm(t, "h-1", t0.Add(-2*time.Minute))
	e.confirm(t, "h-2", t0.Add(-time.Minute))
	e.svc.repo = failingRepo{DeadlineRepository: e.repo, completeErr: errBoom}

	res, err := e.svc.RunSweep(context.Background(), t0)

	if !errors.Is(err, errBoom) || res.Failed != 1 {
		t.Fatalf("got (%+v, %v), want one failure and errBoom", res, err)
	}
	if e.holds.totalCalls() != 1 {
		t.Errorf("Holds was called %d times, want the sweep to stop after the first", e.holds.totalCalls())
	}
}

var errBoom = errors.New("boom")

// stubHolds answers every ExpireHold with the same status.
type stubHolds struct{ status domain.HoldStatus }

func (s *stubHolds) ExpireHold(context.Context, string) (domain.HoldStatus, error) {
	return s.status, nil
}
