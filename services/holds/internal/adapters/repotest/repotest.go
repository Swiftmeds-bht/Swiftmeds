// Package repotest holds the contract tests that every ports.HoldRepository
// must pass. Each adapter's test calls Run with a function that returns an
// empty repository.
package repotest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

// Whole seconds, so databases that keep fewer decimals still pass.
var t0 = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

const (
	testCode = "ABC234"
	noLimit  = 0
)

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
		{"update stores the new status, expiry time and pickup code", updateStores},
		{"update of an unknown hold is not found", updateUnknown},
		{"update with a stale status is a conflict", updateConflict},
		{"only one of many concurrent updates wins", concurrentUpdates},
		{"create stops at the open-hold limit", createStopsAtTheLimit},
		{"a limit of zero means no limit", createWithoutLimit},
		{"the limit is per patient, not per pharmacy", createLimitIsPerPatient},
		{"finished holds do not count against the limit", finishedHoldsDoNotCount},
		{"concurrent creates cannot get past the limit", concurrentCreatesRespectTheLimit},
		{"a pickup code is unique among a pharmacy's active holds", pickupCodesAreUniquePerPharmacy},
		{"list filters by patient, pharmacy and status", listFilters},
		{"list orders ties by id", listTieBreak},
		{"list returns pages after a cursor", listPages},
		{"wrong pickup codes are counted on active holds only", wrongCodesAreCounted},
		{"concurrent wrong pickup codes are all counted", concurrentWrongCodesAllCount},
		{"update keeps the wrong-code count and stores the release mark", updateKeepsAttemptsStoresReleaseMark},
		{"stale pending holds are listed oldest first", staleHoldsAreListed},
		{"holds waiting for a release are listed oldest first", releasePendingHoldsAreListed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRepo(t)) })
	}
}

func newHold(t *testing.T, id string) domain.Hold {
	t.Helper()
	return newHoldFor(t, id, "u-1", "ph-1", t0)
}

func newHoldFor(t *testing.T, id, userID, pharmacyID string, createdAt time.Time) domain.Hold {
	t.Helper()
	h, err := domain.NewHold(id, userID, pharmacyID, "med-1", 2, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// create stores a PENDING hold without a limit.
func create(t *testing.T, repo ports.HoldRepository, h domain.Hold) {
	t.Helper()
	if err := repo.Create(context.Background(), h, noLimit); err != nil {
		t.Fatal(err)
	}
}

// activate makes a stored PENDING hold ACTIVE with the given pickup code.
func activate(t *testing.T, repo ports.HoldRepository, pending domain.Hold, code string) domain.Hold {
	t.Helper()
	active, err := pending.Reserve(pending.CreatedAt.Add(time.Minute), 2*time.Hour, code)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(context.Background(), active, domain.Pending); err != nil {
		t.Fatal(err)
	}
	return active
}

// activeHold creates a hold, makes it ACTIVE and stores it.
func activeHold(t *testing.T, repo ports.HoldRepository, id string) domain.Hold {
	t.Helper()
	pending := newHold(t, id)
	create(t, repo, pending)
	return activate(t, repo, pending, testCode)
}

func ids(holds []domain.Hold) []string {
	out := make([]string, 0, len(holds))
	for _, h := range holds {
		out = append(out, h.ID)
	}
	return out
}

func assertSame(t *testing.T, got, want domain.Hold) {
	t.Helper()
	if got.ID != want.ID || got.UserID != want.UserID || got.PharmacyID != want.PharmacyID ||
		got.ProductID != want.ProductID || got.Quantity != want.Quantity ||
		got.Status != want.Status || got.StatusReason != want.StatusReason ||
		got.PickupCode != want.PickupCode ||
		got.PickupAttempts != want.PickupAttempts || got.StockReleasePending != want.StockReleasePending ||
		!got.CreatedAt.Equal(want.CreatedAt) || !got.ExpiresAt.Equal(want.ExpiresAt) ||
		!got.StatusChangedAt.Equal(want.StatusChangedAt) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func createThenGet(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	want := newHold(t, "h-1")

	if err := repo.Create(ctx, want, noLimit); err != nil {
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
	create(t, repo, first)
	second := first
	second.Quantity = 5

	if err := repo.Create(ctx, second, noLimit); !errors.Is(err, domain.ErrAlreadyExists) {
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
	if got.Status != domain.Active || got.ExpiresAt.IsZero() || got.PickupCode != testCode {
		t.Errorf("status = %s, expires_at = %v, pickup code = %q, want ACTIVE with an expiry time and the code %q",
			got.Status, got.ExpiresAt, got.PickupCode, testCode)
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
				next, _ = active.Collect(t0.Add(time.Hour), "ph-1", testCode)
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

func createStopsAtTheLimit(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	const limit = 3
	for i := range limit {
		h := newHold(t, fmt.Sprintf("h-%d", i))
		if err := repo.Create(ctx, h, limit); err != nil {
			t.Fatalf("hold %d: %v", i, err)
		}
	}

	err := repo.Create(ctx, newHold(t, "h-extra"), limit)

	if !errors.Is(err, domain.ErrLimitReached) {
		t.Fatalf("err = %v, want ErrLimitReached", err)
	}
	if _, err := repo.Get(ctx, "h-extra"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("the refused hold was stored: err = %v, want ErrNotFound", err)
	}
}

func createWithoutLimit(t *testing.T, repo ports.HoldRepository) {
	for i := range 10 {
		create(t, repo, newHold(t, fmt.Sprintf("h-%d", i)))
	}
}

func createLimitIsPerPatient(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	const limit = 2
	for i := range limit {
		h := newHoldFor(t, fmt.Sprintf("a-%d", i), "u-1", "ph-1", t0)
		if err := repo.Create(ctx, h, limit); err != nil {
			t.Fatal(err)
		}
	}

	// Another patient is not affected, but the same patient is also stopped at another pharmacy.
	other := newHoldFor(t, "b-0", "u-2", "ph-1", t0)
	if err := repo.Create(ctx, other, limit); err != nil {
		t.Fatalf("another patient: %v", err)
	}
	sameAtOtherPharmacy := newHoldFor(t, "a-2", "u-1", "ph-2", t0)
	if err := repo.Create(ctx, sameAtOtherPharmacy, limit); !errors.Is(err, domain.ErrLimitReached) {
		t.Fatalf("same patient at another pharmacy: err = %v, want ErrLimitReached (the limit is per patient)", err)
	}
}

// Only PENDING and ACTIVE holds count. Cancelling or collecting frees a place.
func finishedHoldsDoNotCount(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	const limit = 3
	add := func(id string) domain.Hold {
		t.Helper()
		h := newHold(t, id)
		if err := repo.Create(ctx, h, limit); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		return h
	}
	refused := func(id string) {
		t.Helper()
		if err := repo.Create(ctx, newHold(t, id), limit); !errors.Is(err, domain.ErrLimitReached) {
			t.Fatalf("create %s: err = %v, want ErrLimitReached", id, err)
		}
	}

	first, second := add("h-0"), add("h-1")
	add("h-2")
	active := activate(t, repo, second, testCode)
	refused("h-3") // PENDING and ACTIVE holds both count

	cancelled, err := first.Cancel(t0.Add(time.Minute), "changed my mind")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, cancelled, domain.Pending); err != nil {
		t.Fatal(err)
	}
	add("h-3") // a cancelled hold freed a place
	refused("h-4")

	collected, err := active.Collect(t0.Add(time.Hour), "ph-1", testCode)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, collected, domain.Active); err != nil {
		t.Fatal(err)
	}
	add("h-4") // a collected hold freed a place
	refused("h-5")
}

func concurrentCreatesRespectTheLimit(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	const callers, limit = 20, 3
	holds := make([]domain.Hold, callers)
	for i := range callers {
		holds[i] = newHold(t, fmt.Sprintf("h-%d", i))
	}

	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repo.Create(ctx, holds[i], limit)
		}()
	}
	wg.Wait()
	close(errs)

	created, refused := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, domain.ErrLimitReached):
			refused++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if created != limit || refused != callers-limit {
		t.Fatalf("%d created and %d refused, want %d and %d", created, refused, limit, callers-limit)
	}
	stored, err := repo.List(ctx, domain.ListFilter{UserID: "u-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != limit {
		t.Errorf("%d holds stored, want %d", len(stored), limit)
	}
}

func pickupCodesAreUniquePerPharmacy(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	first := newHoldFor(t, "h-1", "u-1", "ph-1", t0)
	create(t, repo, first)
	activeFirst := activate(t, repo, first, testCode)

	// The same code at the same pharmacy is refused, and the hold stays PENDING.
	second := newHoldFor(t, "h-2", "u-2", "ph-1", t0)
	create(t, repo, second)
	clash, err := second.Reserve(t0.Add(time.Minute), 2*time.Hour, testCode)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, clash, domain.Pending); !errors.Is(err, domain.ErrPickupCodeTaken) {
		t.Fatalf("same pharmacy: err = %v, want ErrPickupCodeTaken", err)
	}
	got, _ := repo.Get(ctx, "h-2")
	assertSame(t, got, second)

	// The same code at another pharmacy is fine.
	third := newHoldFor(t, "h-3", "u-3", "ph-2", t0)
	create(t, repo, third)
	activate(t, repo, third, testCode)

	// Once the first hold is collected, its code is free again.
	collected, err := activeFirst.Collect(t0.Add(time.Hour), "ph-1", testCode)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, collected, domain.Active); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, clash, domain.Pending); err != nil {
		t.Fatalf("after the first hold was collected: %v", err)
	}
}

func listFilters(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	create(t, repo, newHoldFor(t, "h-1", "u-1", "ph-1", t0))
	create(t, repo, newHoldFor(t, "h-2", "u-1", "ph-2", t0.Add(time.Minute)))
	create(t, repo, newHoldFor(t, "h-3", "u-2", "ph-1", t0.Add(2*time.Minute)))
	fourth := newHoldFor(t, "h-4", "u-1", "ph-1", t0.Add(3*time.Minute))
	create(t, repo, fourth)
	cancelled, err := fourth.Cancel(t0.Add(4*time.Minute), "changed my mind")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, cancelled, domain.Pending); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		filter domain.ListFilter
		want   []string
	}{
		{"by patient, newest first", domain.ListFilter{UserID: "u-1"}, []string{"h-4", "h-2", "h-1"}},
		{"by pharmacy, newest first", domain.ListFilter{PharmacyID: "ph-1"}, []string{"h-4", "h-3", "h-1"}},
		{"by patient and status", domain.ListFilter{UserID: "u-1", Status: domain.Pending}, []string{"h-2", "h-1"}},
		{"by patient and another status", domain.ListFilter{UserID: "u-1", Status: domain.Cancelled}, []string{"h-4"}},
		{"by patient with no holds", domain.ListFilter{UserID: "nobody"}, []string{}},
		{"with a limit", domain.ListFilter{UserID: "u-1", Limit: 2}, []string{"h-4", "h-2"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := repo.List(ctx, c.filter)

			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(ids(got), c.want) {
				t.Errorf("ids = %v, want %v", ids(got), c.want)
			}
		})
	}
}

// Holds made at the same moment are listed with the larger ID first.
func listTieBreak(t *testing.T, repo ports.HoldRepository) {
	create(t, repo, newHoldFor(t, "h-a", "u-1", "ph-1", t0))
	create(t, repo, newHoldFor(t, "h-b", "u-1", "ph-1", t0))

	got, err := repo.List(context.Background(), domain.ListFilter{UserID: "u-1"})

	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"h-b", "h-a"}; !slices.Equal(ids(got), want) {
		t.Errorf("ids = %v, want %v", ids(got), want)
	}
}

func listPages(t *testing.T, repo ports.HoldRepository) {
	ctx := context.Background()
	for i := range 5 {
		create(t, repo, newHoldFor(t, fmt.Sprintf("h-%d", i), "u-1", "ph-1", t0.Add(time.Duration(i)*time.Minute)))
	}

	var after *domain.Cursor
	var pages [][]string
	for range 4 {
		page, err := repo.List(ctx, domain.ListFilter{UserID: "u-1", After: after, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, ids(page))
		if len(page) == 0 {
			break
		}
		last := page[len(page)-1]
		after = &domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}

	want := [][]string{{"h-4", "h-3"}, {"h-2", "h-1"}, {"h-0"}, {}}
	if len(pages) != len(want) {
		t.Fatalf("pages = %v, want %v", pages, want)
	}
	for i := range want {
		if !slices.Equal(pages[i], want[i]) {
			t.Errorf("page %d = %v, want %v", i, pages[i], want[i])
		}
	}
}
