// Package repotest holds the contract tests that every ports.FeedbackRepository
// must pass. Each adapter's test calls Run with a function that returns an
// empty repository.
package repotest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/ports"
)

// Whole seconds, so databases that keep fewer decimals still pass.
var t0 = time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

var ctx = context.Background()

// Run calls every contract test with a new, empty repository.
func Run(t *testing.T, newRepo func(*testing.T) ports.FeedbackRepository) {
	t.Helper()
	tests := []struct {
		name string
		run  func(*testing.T, ports.FeedbackRepository)
	}{
		{"save then list returns every field", saveThenList},
		{"save for a hold that has feedback is refused", saveDuplicateHold},
		{"the hook runs once, and only for a save that goes through", hookRunsOnce},
		{"a failing hook stores nothing and its error comes back", hookFails},
		{"only one of many concurrent saves for a hold wins", concurrentSaves},
		{"list shows only the pharmacy's feedback", listFiltersByPharmacy},
		{"list is newest first, ties broken by id", listOrder},
		{"list pages with a cursor", listPages},
		{"list with no limit returns everything", listNoLimit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRepo(t)) })
	}
}

func fb(id, hold, pharmacy string, at time.Time) domain.Feedback {
	return domain.Feedback{
		ID: id, HoldID: hold, PharmacyID: pharmacy, ProductID: "med-1",
		WasInStock: true, Rating: 3, Comment: "ok", CreatedAt: at,
	}
}

func same(a, b domain.Feedback) bool {
	return a.ID == b.ID && a.HoldID == b.HoldID && a.PharmacyID == b.PharmacyID &&
		a.ProductID == b.ProductID && a.WasInStock == b.WasInStock && a.Rating == b.Rating &&
		a.Comment == b.Comment && a.CreatedAt.Equal(b.CreatedAt)
}

func mustSave(t *testing.T, r ports.FeedbackRepository, f domain.Feedback) {
	t.Helper()
	if err := r.Save(ctx, f, nil); err != nil {
		t.Fatalf("Save(%s): %v", f.ID, err)
	}
}

func list(t *testing.T, r ports.FeedbackRepository, f domain.ListFilter) []domain.Feedback {
	t.Helper()
	items, err := r.ListByPharmacy(ctx, f)
	if err != nil {
		t.Fatalf("ListByPharmacy: %v", err)
	}
	return items
}

func order(items []domain.Feedback) string {
	s := ""
	for _, it := range items {
		s += it.ID + " "
	}
	return s
}

func saveThenList(t *testing.T, r ports.FeedbackRepository) {
	want := domain.Feedback{
		ID: "f-1", HoldID: "h-1", PharmacyID: "ph-1", ProductID: "med-9",
		WasInStock: false, Rating: 0, Comment: "སྨན་མེད། no stock, wrong shelf", CreatedAt: t0,
	}
	mustSave(t, r, want)

	got := list(t, r, domain.ListFilter{PharmacyID: "ph-1"})

	if len(got) != 1 || !same(got[0], want) {
		t.Fatalf("list = %+v, want [%+v]", got, want)
	}
	if got[0].CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt location = %v, want UTC", got[0].CreatedAt.Location())
	}
}

func saveDuplicateHold(t *testing.T, r ports.FeedbackRepository) {
	first := fb("f-1", "h-1", "ph-1", t0)
	mustSave(t, r, first)

	err := r.Save(ctx, fb("f-2", "h-1", "ph-1", t0.Add(time.Minute)), nil)

	if !errors.Is(err, domain.ErrAlreadySubmitted) {
		t.Errorf("err = %v, want ErrAlreadySubmitted", err)
	}
	got := list(t, r, domain.ListFilter{PharmacyID: "ph-1"})
	if len(got) != 1 || !same(got[0], first) {
		t.Errorf("list = %+v, want only the first feedback", got)
	}
}

func hookRunsOnce(t *testing.T, r ports.FeedbackRepository) {
	calls := 0
	hook := func(context.Context) error { calls++; return nil }

	if err := r.Save(ctx, fb("f-1", "h-1", "ph-1", t0), hook); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("hook ran %d times after a good save, want 1", calls)
	}

	err := r.Save(ctx, fb("f-2", "h-1", "ph-1", t0), hook)
	if !errors.Is(err, domain.ErrAlreadySubmitted) {
		t.Errorf("err = %v, want ErrAlreadySubmitted", err)
	}
	if calls != 1 {
		t.Errorf("hook ran %d times in total, want 1: a refused save must not run it", calls)
	}
}

func hookFails(t *testing.T, r ports.FeedbackRepository) {
	boom := errors.New("event bus is down")

	err := r.Save(ctx, fb("f-1", "h-1", "ph-1", t0), func(context.Context) error { return boom })

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the hook's error", err)
	}
	if got := list(t, r, domain.ListFilter{PharmacyID: "ph-1"}); len(got) != 0 {
		t.Errorf("list = %+v, want nothing stored", got)
	}
	// The hold is free again: the retry goes through.
	if err := r.Save(ctx, fb("f-2", "h-1", "ph-1", t0), nil); err != nil {
		t.Errorf("save after a failed hook: %v", err)
	}
}

func concurrentSaves(t *testing.T, r ports.FeedbackRepository) {
	const workers = 8
	var hookCalls, wins atomic.Int32
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = r.Save(ctx, fb(fmt.Sprintf("f-%d", i), "h-1", "ph-1", t0),
				func(context.Context) error { hookCalls.Add(1); return nil })
			if errs[i] == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()

	if wins.Load() != 1 {
		t.Errorf("%d saves succeeded, want exactly 1", wins.Load())
	}
	if hookCalls.Load() != 1 {
		t.Errorf("hook ran %d times, want exactly 1", hookCalls.Load())
	}
	for i, err := range errs {
		if err != nil && !errors.Is(err, domain.ErrAlreadySubmitted) {
			t.Errorf("save %d: err = %v, want nil or ErrAlreadySubmitted", i, err)
		}
	}
	if got := list(t, r, domain.ListFilter{PharmacyID: "ph-1"}); len(got) != 1 {
		t.Errorf("stored %d feedback for one hold, want 1", len(got))
	}
}

func listFiltersByPharmacy(t *testing.T, r ports.FeedbackRepository) {
	mustSave(t, r, fb("f-1", "h-1", "ph-1", t0))
	mustSave(t, r, fb("f-2", "h-2", "ph-2", t0))

	if got := list(t, r, domain.ListFilter{PharmacyID: "ph-1"}); order(got) != "f-1 " {
		t.Errorf("ph-1 = %s, want f-1", order(got))
	}
	if got := list(t, r, domain.ListFilter{PharmacyID: "ph-3"}); len(got) != 0 {
		t.Errorf("unknown pharmacy = %s, want nothing", order(got))
	}
}

func listOrder(t *testing.T, r ports.FeedbackRepository) {
	mustSave(t, r, fb("f-a", "h-1", "ph-1", t0))
	mustSave(t, r, fb("f-c", "h-2", "ph-1", t0.Add(time.Minute)))
	mustSave(t, r, fb("f-b", "h-3", "ph-1", t0.Add(time.Minute)))

	got := list(t, r, domain.ListFilter{PharmacyID: "ph-1"})

	if order(got) != "f-c f-b f-a " {
		t.Errorf("order = %s, want f-c f-b f-a (newest first, then id descending)", order(got))
	}
}

func listPages(t *testing.T, r ports.FeedbackRepository) {
	for i := 1; i <= 5; i++ {
		mustSave(t, r, fb(fmt.Sprintf("f-%d", i), fmt.Sprintf("h-%d", i), "ph-1", t0.Add(time.Duration(i)*time.Minute)))
	}

	var seen string
	var after *domain.Cursor
	for page := 1; page <= 4; page++ {
		items := list(t, r, domain.ListFilter{PharmacyID: "ph-1", After: after, Limit: 2})
		if len(items) == 0 {
			break
		}
		seen += order(items)
		last := items[len(items)-1]
		after = &domain.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}

	if seen != "f-5 f-4 f-3 f-2 f-1 " {
		t.Errorf("pages gave %s, want every feedback once, newest first", seen)
	}
}

func listNoLimit(t *testing.T, r ports.FeedbackRepository) {
	for i := 1; i <= 7; i++ {
		mustSave(t, r, fb(fmt.Sprintf("f-%d", i), fmt.Sprintf("h-%d", i), "ph-1", t0.Add(time.Duration(i)*time.Minute)))
	}

	if got := list(t, r, domain.ListFilter{PharmacyID: "ph-1", Limit: 0}); len(got) != 7 {
		t.Errorf("got %d feedback, want 7", len(got))
	}
}
