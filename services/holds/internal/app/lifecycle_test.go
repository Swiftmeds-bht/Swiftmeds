package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

// newHookedService builds a Service whose repository can run a function just
// before an Update, to play out two callers racing for the same hold.
func newHookedService(t *testing.T) (*Service, *hookRepo, *clock) {
	t.Helper()
	repo := &hookRepo{HoldRepository: memory.NewRepository()}
	clk := &clock{t: t0}
	n := 0
	ids := func() (string, error) {
		n++
		return "h-" + string(rune('0'+n)), nil
	}
	svc := NewService(repo, testConfig(), clk.now, WithIDGenerator(ids), WithCodeGenerator(always(testCode)))
	return svc, repo, clk
}

func TestReserveHoldMakesAPickupCodeAndFixesTheExpiryTime(t *testing.T) {
	ctx := context.Background()
	svc, _, clk := newTestService(t)
	h, _ := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 2)
	clk.advance(30 * time.Second) // Inventory answers a little after the hold was created

	active, err := svc.ReserveHold(ctx, h.ID)

	if err != nil {
		t.Fatal(err)
	}
	if active.Status != domain.Active || active.PickupCode != testCode {
		t.Errorf("status = %s, pickup code = %q, want ACTIVE with %q", active.Status, active.PickupCode, testCode)
	}
	if want := t0.Add(30*time.Second + 2*time.Hour); !active.ExpiresAt.Equal(want) {
		t.Errorf("expires_at = %v, want %v", active.ExpiresAt, want)
	}
	if stored := mustGet(t, svc, h.ID); stored.PickupCode != testCode || stored.Status != domain.Active {
		t.Errorf("stored hold = %+v, want the ACTIVE hold with its code", stored)
	}
}

func TestReserveHoldOnlyWorksOnAPendingHold(t *testing.T) {
	svc, _, _ := newTestService(t)
	active := activeHold(t, svc)

	_, err := svc.ReserveHold(context.Background(), active.ID)

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestReserveHoldOfAnUnknownHold(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.ReserveHold(context.Background(), "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestReserveHoldTriesANewCodeWhenAnotherHoldHasIt(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t, WithCodeGenerator(sequence("AAAAAA", "AAAAAA", "BBBBBB")))
	first, _ := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1)
	second, _ := svc.CreateHold(ctx, "u-2", "ph-1", "med-1", 1)
	if _, err := svc.ReserveHold(ctx, first.ID); err != nil {
		t.Fatal(err)
	}

	active, err := svc.ReserveHold(ctx, second.ID)

	if err != nil {
		t.Fatal(err)
	}
	if active.PickupCode != "BBBBBB" {
		t.Errorf("pickup code = %q, want BBBBBB after two clashes with AAAAAA", active.PickupCode)
	}
}

func TestReserveHoldGivesUpAfterTooManyClashes(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t, WithCodeGenerator(always("AAAAAA")))
	first, _ := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1)
	second, _ := svc.CreateHold(ctx, "u-2", "ph-1", "med-1", 1)
	if _, err := svc.ReserveHold(ctx, first.ID); err != nil {
		t.Fatal(err)
	}

	_, err := svc.ReserveHold(ctx, second.ID)

	if !errors.Is(err, domain.ErrPickupCodeTaken) {
		t.Fatalf("err = %v, want ErrPickupCodeTaken", err)
	}
	if got := mustGet(t, svc, second.ID); got.Status != domain.Pending {
		t.Errorf("status = %s, want the hold to stay PENDING", got.Status)
	}
}

func TestReserveHoldFailsWhenNoCodeCanBeMade(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t, WithCodeGenerator(func() (string, error) { return "", errBoom }))
	h, _ := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1)

	if _, err := svc.ReserveHold(ctx, h.ID); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestReserveHoldReportsAnUpdateError(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newHookedService(t)
	h, _ := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1)
	repo.beforeUpdate = func() { _, _ = svc.CancelHold(ctx, h.ID, "u-1", "changed my mind") }

	_, err := svc.ReserveHold(ctx, h.ID)

	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict (the hold was cancelled in between)", err)
	}
}

func TestRejectHold(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	pending, _ := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1)

	rejected, err := svc.RejectHold(ctx, pending.ID, "no stock")

	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != domain.Rejected || rejected.StatusReason != "no stock" {
		t.Errorf("status = %s, reason = %q, want REJECTED with the reason", rejected.Status, rejected.StatusReason)
	}
	if _, err := svc.RejectHold(ctx, pending.ID, "again"); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("rejecting twice: err = %v, want ErrInvalidTransition", err)
	}
}

func TestCollectHoldWithTheRightCode(t *testing.T) {
	svc, _, clk := newTestService(t)
	active := activeHold(t, svc)
	clk.advance(time.Hour)

	collected, err := svc.CollectHold(context.Background(), active.ID, "ph-1", testCode)

	if err != nil {
		t.Fatal(err)
	}
	if collected.Status != domain.Collected || !collected.StatusChangedAt.Equal(clk.now()) {
		t.Errorf("status = %s changed at %v, want COLLECTED changed at %v", collected.Status, collected.StatusChangedAt, clk.now())
	}
	if got := mustGet(t, svc, active.ID); got.Status != domain.Collected {
		t.Errorf("stored status = %s, want COLLECTED", got.Status)
	}
}

// The ticket: "Wrong pickup code is rejected".
func TestCollectHoldRejectsAWrongPickupCodeAndStaysActive(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	active := activeHold(t, svc)

	for _, wrong := range []string{"ZZZ999", "ABC235", "abc23", ""} {
		_, err := svc.CollectHold(ctx, active.ID, "ph-1", wrong)

		if err == nil {
			t.Fatalf("code %q was accepted", wrong)
		}
		if !errors.Is(err, domain.ErrWrongPickupCode) && !errors.Is(err, domain.ErrMissingPickupCode) {
			t.Errorf("code %q: err = %v, want ErrWrongPickupCode or ErrMissingPickupCode", wrong, err)
		}
		if got := mustGet(t, svc, active.ID); got.Status != domain.Active {
			t.Fatalf("code %q: status = %s, want the hold to stay ACTIVE", wrong, got.Status)
		}
	}
	if _, err := svc.CollectHold(ctx, active.ID, "ph-1", testCode); err != nil {
		t.Errorf("the right code after wrong ones: %v", err)
	}
}

func TestCollectHoldAcceptsTheCodeTheWayAPersonTypesIt(t *testing.T) {
	svc, _, _ := newTestService(t)
	active := activeHold(t, svc)

	if _, err := svc.CollectHold(context.Background(), active.ID, "ph-1", " abc-234 "); err != nil {
		t.Fatalf("err = %v, want the code accepted in lower case with a dash and spaces", err)
	}
}

// The ticket: "collecting twice fails".
func TestCollectingTwiceFails(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	active := activeHold(t, svc)
	if _, err := svc.CollectHold(ctx, active.ID, "ph-1", testCode); err != nil {
		t.Fatal(err)
	}

	_, err := svc.CollectHold(ctx, active.ID, "ph-1", testCode)

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("second collect: err = %v, want ErrInvalidTransition", err)
	}
	if got := mustGet(t, svc, active.ID); got.Status != domain.Collected {
		t.Errorf("status = %s, want COLLECTED", got.Status)
	}
}

func TestOnlyOneOfManyCollectsAtTheSameTimeWins(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	active := activeHold(t, svc)
	const callers = 20

	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.CollectHold(ctx, active.ID, "ph-1", testCode)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	wins := 0
	for err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrInvalidTransition):
			// lost the race, or read the hold after it was collected
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d collects succeeded, want exactly 1", wins)
	}
}

func TestCollectHoldRefusesAnotherPharmacy(t *testing.T) {
	svc, _, _ := newTestService(t)
	active := activeHold(t, svc)

	_, err := svc.CollectHold(context.Background(), active.ID, "ph-2", testCode)

	if !errors.Is(err, domain.ErrWrongPharmacy) {
		t.Fatalf("err = %v, want ErrWrongPharmacy", err)
	}
}

func TestCollectHoldRefusesAHoldThatRanOutOfTime(t *testing.T) {
	svc, _, clk := newTestService(t)
	active := activeHold(t, svc)
	clk.advance(2 * time.Hour) // exactly the hold duration

	_, err := svc.CollectHold(context.Background(), active.ID, "ph-1", testCode)

	if !errors.Is(err, domain.ErrHoldExpired) {
		t.Fatalf("err = %v, want ErrHoldExpired", err)
	}
	if got := mustGet(t, svc, active.ID); got.Status != domain.Active {
		t.Errorf("status = %s, want ACTIVE until Expiry calls ExpireHold", got.Status)
	}
}

func TestCollectHoldOfAnUnknownHold(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.CollectHold(context.Background(), "nope", "ph-1", testCode); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCollectHoldLosingARaceIsAConflict(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newHookedService(t)
	active := activeHold(t, svc)
	repo.beforeUpdate = func() { _, _ = svc.CancelHold(ctx, active.ID, "u-1", "changed my mind") }

	_, err := svc.CollectHold(ctx, active.ID, "ph-1", testCode)

	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	if got := mustGet(t, svc, active.ID); got.Status != domain.Cancelled {
		t.Errorf("status = %s, want the cancel (which came first) to stand", got.Status)
	}
}

func TestCancelHold(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	active := activeHold(t, svc)

	cancelled, err := svc.CancelHold(ctx, active.ID, "u-1", "changed my mind")

	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != domain.Cancelled || cancelled.StatusReason != "changed my mind" {
		t.Errorf("status = %s, reason = %q, want CANCELLED with the reason", cancelled.Status, cancelled.StatusReason)
	}
	if _, err := svc.CancelHold(ctx, active.ID, "u-1", "again"); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("cancelling twice: err = %v, want ErrInvalidTransition", err)
	}
}

func TestCancelHoldIsOnlyForThePatientWhoMadeIt(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	active := activeHold(t, svc)

	if _, err := svc.CancelHold(ctx, active.ID, "u-2", "not mine"); !errors.Is(err, domain.ErrNotHoldOwner) {
		t.Errorf("another patient: err = %v, want ErrNotHoldOwner", err)
	}
	if _, err := svc.CancelHold(ctx, active.ID, "", "no user"); !errors.Is(err, domain.ErrMissingUserID) {
		t.Errorf("no patient: err = %v, want ErrMissingUserID", err)
	}
	if got := mustGet(t, svc, active.ID); got.Status != domain.Active {
		t.Errorf("status = %s, want the hold to stay ACTIVE", got.Status)
	}
}

func TestCancelHoldAlsoWorksOnAPendingHold(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	pending, _ := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1)

	cancelled, err := svc.CancelHold(ctx, pending.ID, "u-1", "changed my mind")

	if err != nil || cancelled.Status != domain.Cancelled {
		t.Fatalf("got (%+v, %v), want a CANCELLED hold", cancelled, err)
	}
}

func TestExpireHold(t *testing.T) {
	ctx := context.Background()
	svc, _, clk := newTestService(t)
	active := activeHold(t, svc)

	if _, err := svc.ExpireHold(ctx, active.ID); !errors.Is(err, domain.ErrNotYetExpired) {
		t.Fatalf("before expires_at: err = %v, want ErrNotYetExpired", err)
	}
	if got := mustGet(t, svc, active.ID); got.Status != domain.Active {
		t.Fatalf("status = %s, want the hold to stay ACTIVE", got.Status)
	}

	clk.advance(2 * time.Hour)
	expired, err := svc.ExpireHold(ctx, active.ID)

	if err != nil || expired.Status != domain.Expired {
		t.Fatalf("at expires_at: got (%+v, %v), want an EXPIRED hold", expired, err)
	}
	again, err := svc.ExpireHold(ctx, active.ID)
	if err != nil || again.Status != domain.Expired {
		t.Errorf("expiring twice: got (%+v, %v), want the EXPIRED hold and no error", again, err)
	}
}

func TestExpireHoldDoesNothingUnlessTheHoldIsActive(t *testing.T) {
	ctx := context.Background()
	svc, _, clk := newTestService(t)
	pending, _ := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1)
	collected := activeHold(t, svc)
	if _, err := svc.CollectHold(ctx, collected.ID, "ph-1", testCode); err != nil {
		t.Fatal(err)
	}
	clk.advance(24 * time.Hour)

	for _, id := range []string{pending.ID, collected.ID} {
		before := mustGet(t, svc, id)

		got, err := svc.ExpireHold(ctx, id)

		if err != nil || got.Status != before.Status {
			t.Errorf("%s: got (%s, %v), want the hold unchanged (%s) and no error", id, got.Status, err, before.Status)
		}
	}
	if _, err := svc.ExpireHold(ctx, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown hold: err = %v, want ErrNotFound", err)
	}
}

func TestExpireHoldThatWasCollectedAtTheSameMomentIsLeftAlone(t *testing.T) {
	ctx := context.Background()
	svc, repo, clk := newHookedService(t)
	active := activeHold(t, svc)
	clk.advance(2 * time.Hour)
	// Expiry reads the hold, then the patient's pickup is stored just before Expiry's own write.
	repo.beforeUpdate = func() {
		clk.advance(-time.Minute) // the pickup happened a minute before expires_at
		_, _ = svc.CollectHold(ctx, active.ID, "ph-1", testCode)
		clk.advance(time.Minute)
	}

	got, err := svc.ExpireHold(ctx, active.ID)

	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.Collected {
		t.Errorf("status = %s, want COLLECTED (the collect won)", got.Status)
	}
}

func TestExpireHoldReportsARepositoryError(t *testing.T) {
	svc := NewService(errRepo{}, testConfig(), (&clock{t: t0}).now)

	if _, err := svc.ExpireHold(context.Background(), "h-1"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestUpdateErrorsOtherThanAConflictAreReported(t *testing.T) {
	ctx := context.Background()
	svc, repo, clk := newHookedService(t)
	active := activeHold(t, svc)
	clk.advance(2 * time.Hour)
	// From here on, every write to the repository fails.
	repo.HoldRepository = failUpdate{repo.HoldRepository}

	_, err := svc.ExpireHold(ctx, active.ID)

	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

// failUpdate wraps a repository and makes every Update fail.
type failUpdate struct{ ports.HoldRepository }

func (failUpdate) Update(context.Context, domain.Hold, domain.Status) error { return errBoom }
