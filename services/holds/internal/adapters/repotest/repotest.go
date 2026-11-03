// Package repotest holds the contract tests that every ports.HoldRepository
// must pass. Each adapter's test calls Run with a function that returns an
// empty repository.
package repotest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

// Whole seconds, so databases that keep fewer decimals still pass.
var t0 = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

// Run calls every contract test with a new, empty repository.
func Run(t *testing.T, newRepo func(*testing.T) ports.HoldRepository) {
	t.Helper()
	tests := []struct {
		name string
		run  func(*testing.T, ports.HoldRepository)
	}{
		{"create then get returns the same hold", createThenGet},
		{"create rejects a duplicate id", createDuplicate},
		{"get of an unknown id is not found", getUnknown},
		{"update stores the new status and expiry time", updateStores},
		{"update of an unknown hold is not found", updateUnknown},
		{"update with a stale status is a conflict", updateConflict},
		{"only one of many concurrent updates wins", concurrentUpdates},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRepo(t)) })
	}
}

func newHold(t *testing.T, id string) domain.Hold {
	t.Helper()
	h, err := domain.NewHold(id, "u-1", "ph-1", "med-1", 2, t0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// activeHold creates a hold, makes it ACTIVE and stores it.
func activeHold(t *testing.T, repo ports.HoldRepository, id string) domain.Hold {
	t.Helper()
	ctx := context.Background()
	pending := newHold(t, id)
	if err := repo.Create(ctx, pending); err != nil {
		t.Fatal(err)
	}
	active, err := pending.Reserve(t0.Add(time.Minute), 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, active, domain.Pending); err != nil {
		t.Fatal(err)
	}
	return active
}

func assertSame(t *testing.T, got, want domain.Hold) {
	t.Helper()
	if got.ID != want.ID || got.UserID != want.UserID || got.PharmacyID != want.PharmacyID ||
		got.ProductID != want.ProductID || got.Quantity != want.Quantity ||
		got.Status != want.Status || got.StatusReason != want.StatusReason ||
		!got.CreatedAt.Equal(want.CreatedAt) || !got.ExpiresAt.Equal(want.ExpiresAt) ||
		!got.StatusChangedAt.Equal(want.StatusChangedAt) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func createThenGet(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	want := newHold(t, "h-1")

	if err := repo.Create(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "h-1")

	if err != nil {
		t.Fatal(err)
	}
	assertSame(t, got, want)
}

func createDuplicate(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	first := newHold(t, "h-1")
	if err := repo.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Quantity = 5

	if err := repo.Create(ctx, second); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("err = %v, want ErrAlreadyExists", err)
	}
	got, _ := repo.Get(ctx, "h-1")
	assertSame(t, got, first)
}

func getUnknown(t *testing.T, repo ports.HoldRepository) {
	if _, err := repo.Get(context.Background(), "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func updateStores(t *testing.T, repo ports.HoldRepository) {
	active := activeHold(t, repo, "h-1")

	got, err := repo.Get(context.Background(), "h-1")

	if err != nil {
		t.Fatal(err)
	}
	assertSame(t, got, active)
	if got.Status != domain.Active || got.ExpiresAt.IsZero() {
		t.Errorf("status = %s, expires_at = %v, want ACTIVE with an expiry time", got.Status, got.ExpiresAt)
	}
}

func updateUnknown(t *testing.T, repo ports.HoldRepository) {
	err := repo.Update(context.Background(), newHold(t, "nope"), domain.Pending)

	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func updateConflict(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	active := activeHold(t, repo, "h-1")
	// A slow caller still thinks the hold is PENDING and tries to cancel it.
	staleCancel, err := newHold(t, "h-1").Cancel(t0.Add(2*time.Minute), "changed my mind")
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Update(ctx, staleCancel, domain.Pending); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	got, _ := repo.Get(ctx, "h-1")
	assertSame(t, got, active)
}

// Collect and Expire (and more) arrive at the same moment for one ACTIVE hold.
// Exactly one may win, and the stored status must be the winner's.
func concurrentUpdates(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	active := activeHold(t, repo, "h-1")
	const callers = 50

	type result struct {
		err    error
		target domain.Status
	}
	results := make(chan result, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var next domain.Hold
			if i%2 == 0 {
				next, _ = active.Collect(t0.Add(time.Hour))
			} else {
				next, _ = active.Expire(t0.Add(time.Hour))
			}
			results <- result{err: repo.Update(ctx, next, domain.Active), target: next.Status}
		}()
	}
	wg.Wait()
	close(results)

	wins, conflicts := 0, 0
	var winner domain.Status
	for r := range results {
		switch {
		case r.err == nil:
			wins++
			winner = r.target
		case errors.Is(r.err, domain.ErrConflict):
			conflicts++
		default:
			t.Errorf("unexpected error: %v", r.err)
		}
	}
	if wins != 1 || conflicts != callers-1 {
		t.Fatalf("%d wins and %d conflicts, want 1 and %d", wins, conflicts, callers-1)
	}
	if got, _ := repo.Get(ctx, "h-1"); got.Status != winner {
		t.Errorf("stored status = %s, want the winner's %s", got.Status, winner)
	}
}
