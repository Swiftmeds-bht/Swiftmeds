package repotest

// Contract tests for the SCRUM-125 repository methods: the wrong-pickup-code
// count, and the lists the stale-hold sweep reads.

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

func wrongCodesAreCounted(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	active := activeHold(t, repo, "h-active")
	for want := 1; want <= 3; want++ {
		got, err := repo.RecordWrongPickupCode(ctx, active.ID)
		if err != nil || got != want {
			t.Fatalf("record %d = %d, %v", want, got, err)
		}
	}
	if stored, _ := repo.Get(ctx, active.ID); stored.PickupAttempts != 3 {
		t.Fatalf("stored attempts = %d, want 3", stored.PickupAttempts)
	}

	pending := newHold(t, "h-pending")
	create(t, repo, pending)
	if _, err := repo.RecordWrongPickupCode(ctx, pending.ID); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("pending hold: err = %v, want ErrConflict", err)
	}
	if _, err := repo.RecordWrongPickupCode(ctx, "h-unknown"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown hold: err = %v, want ErrNotFound", err)
	}
}

func concurrentWrongCodesAllCount(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	h := activeHold(t, repo, "h-1")
	const tries = 20
	var wg sync.WaitGroup
	errs := make(chan error, tries)
	for range tries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.RecordWrongPickupCode(ctx, h.ID); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if stored, _ := repo.Get(ctx, h.ID); stored.PickupAttempts != tries {
		t.Fatalf("attempts = %d, want %d: concurrent wrong codes were lost", stored.PickupAttempts, tries)
	}
}

func updateKeepsAttemptsStoresReleaseMark(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	active := activeHold(t, repo, "h-1")
	if _, err := repo.RecordWrongPickupCode(ctx, active.ID); err != nil {
		t.Fatal(err)
	}
	// The caller still holds the copy from before the wrong code: Update must not undo the count.
	expired, err := active.Expire(active.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, expired, domain.Active); err != nil {
		t.Fatal(err)
	}
	if stored, _ := repo.Get(ctx, active.ID); stored.PickupAttempts != 1 {
		t.Errorf("attempts after update = %d, want 1", stored.PickupAttempts)
	}

	pending := newHold(t, "h-2")
	create(t, repo, pending)
	rejected, err := pending.Reject(t0.Add(time.Hour), "timed out")
	if err != nil {
		t.Fatal(err)
	}
	rejected.StockReleasePending = true
	if err := repo.Update(ctx, rejected, domain.Pending); err != nil {
		t.Fatal(err)
	}
	stored, _ := repo.Get(ctx, pending.ID)
	assertSame(t, stored, rejected)

	cleared := stored
	cleared.StockReleasePending = false
	if err := repo.Update(ctx, cleared, domain.Rejected); err != nil {
		t.Fatal(err)
	}
	if stored, _ := repo.Get(ctx, pending.ID); stored.StockReleasePending {
		t.Error("release mark not cleared")
	}
}

func staleHoldsAreListed(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	create(t, repo, newHoldFor(t, "h-new", "u-1", "ph-1", t0.Add(30*time.Minute)))
	create(t, repo, newHoldFor(t, "h-b", "u-2", "ph-1", t0))
	create(t, repo, newHoldFor(t, "h-old", "u-3", "ph-1", t0.Add(-time.Hour)))
	create(t, repo, newHoldFor(t, "h-a", "u-4", "ph-1", t0))
	activate(t, repo, newHoldForCreate(t, repo, "h-active", t0.Add(-2*time.Hour)), testCode)

	cutoff := t0.Add(10 * time.Minute)
	got, err := repo.ListStalePending(ctx, cutoff, 10)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"h-old", "h-a", "h-b"}; !slices.Equal(ids(got), want) {
		t.Fatalf("stale = %v, want %v (PENDING only, before the cutoff, oldest first, then by id)", ids(got), want)
	}
	got, err = repo.ListStalePending(ctx, cutoff, 2)
	if err != nil || !slices.Equal(ids(got), []string{"h-old", "h-a"}) {
		t.Fatalf("limit 2 = %v, %v", ids(got), err)
	}
	if got, _ := repo.ListStalePending(ctx, t0, 10); !slices.Equal(ids(got), []string{"h-old"}) {
		t.Errorf("cutoff t0 = %v, want only h-old: created exactly at the cutoff is not stale", ids(got))
	}
}

// newHoldForCreate creates a PENDING hold for its own patient at createdAt and returns it.
func newHoldForCreate(t *testing.T, repo ports.HoldRepository, id string, createdAt time.Time) domain.Hold {
	t.Helper()
	h := newHoldFor(t, id, "u-"+id, "ph-1", createdAt)
	create(t, repo, h)
	return h
}

func releasePendingHoldsAreListed(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	mark := func(id string, at time.Time) {
		t.Helper()
		pending := newHoldForCreate(t, repo, id, t0)
		rejected, err := pending.Reject(at, "timed out")
		if err != nil {
			t.Fatal(err)
		}
		rejected.StockReleasePending = true
		if err := repo.Update(ctx, rejected, domain.Pending); err != nil {
			t.Fatal(err)
		}
	}
	mark("h-late", t0.Add(3*time.Hour))
	mark("h-b", t0.Add(time.Hour))
	mark("h-a", t0.Add(time.Hour))
	newHoldForCreate(t, repo, "h-plain", t0) // not marked

	got, err := repo.ListReleasePending(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"h-a", "h-b", "h-late"}; !slices.Equal(ids(got), want) {
		t.Fatalf("release pending = %v, want %v", ids(got), want)
	}
	if got, _ := repo.ListReleasePending(ctx, 1); !slices.Equal(ids(got), []string{"h-a"}) {
		t.Errorf("limit 1 = %v", ids(got))
	}
}
