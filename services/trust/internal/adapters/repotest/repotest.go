// Package repotest is the contract test suite for ports.TrustRepository.
// Every adapter runs it, so the in-memory and Postgres repositories are
// guaranteed to behave the same way:
//
//	repotest.Run(t, func(t *testing.T) ports.TrustRepository { return memory.NewRepository() })
package repotest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/ports"
)

// Factory returns an empty repository for one subtest.
type Factory func(t *testing.T) ports.TrustRepository

var t0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

func fb(id, pharmacy string, at time.Time) domain.Feedback {
	return domain.Feedback{ID: id, PharmacyID: pharmacy, WasInStock: true, OccurredAt: at}
}

// set returns an update that ignores the stored record and saves want.
func set(want domain.Record) func(domain.Record) domain.Record {
	return func(domain.Record) domain.Record { return want }
}

// count returns an update that adds one feedback and records what it was given.
func count(seen *[]domain.Record) func(domain.Record) domain.Record {
	return func(r domain.Record) domain.Record {
		*seen = append(*seen, r)
		r.FeedbackCount++
		return r
	}
}

// Run executes every contract case against repositories made by newRepo.
func Run(t *testing.T, newRepo Factory) {
	ctx := context.Background()

	t.Run("a pharmacy without feedback has no record", func(t *testing.T) {
		repo := newRepo(t)
		got := mustRecords(t, repo, "ph-1", "ph-2")
		if len(got) != 0 {
			t.Fatalf("Records = %+v, want none", got)
		}
		if got := mustRecords(t, repo); len(got) != 0 {
			t.Fatalf("Records() = %+v, want none", got)
		}
	})

	t.Run("the first feedback starts from an empty record and is saved", func(t *testing.T) {
		repo := newRepo(t)
		var seen []domain.Record
		mustApply(t, repo, fb("fb-1", "ph-1", t0), count(&seen), true)
		if len(seen) != 1 || seen[0] != (domain.Record{PharmacyID: "ph-1"}) {
			t.Fatalf("update was given %+v, want one empty record for ph-1", seen)
		}
		got := mustRecords(t, repo, "ph-1")
		if got["ph-1"].FeedbackCount != 1 {
			t.Errorf("saved record = %+v", got["ph-1"])
		}
	})

	t.Run("every field round-trips", func(t *testing.T) {
		repo := newRepo(t)
		want := domain.Record{
			PharmacyID: "ph-1", FeedbackCount: 7, InStockCount: 5,
			WeightedInStock: 3.141592653589793, WeightedTotal: 4.000000000000001, LastAt: t0.Add(90 * time.Minute),
		}
		mustApply(t, repo, fb("fb-1", "ph-1", t0), set(want), true)
		got := mustRecords(t, repo, "ph-1")["ph-1"]
		if !same(got, want) {
			t.Errorf("Records = %+v\nwant      %+v", got, want)
		}
	})

	t.Run("the next feedback is given the stored record", func(t *testing.T) {
		repo := newRepo(t)
		var seen []domain.Record
		mustApply(t, repo, fb("fb-1", "ph-1", t0), count(&seen), true)
		mustApply(t, repo, fb("fb-2", "ph-1", t0.Add(time.Hour)), count(&seen), true)
		if len(seen) != 2 || seen[1].FeedbackCount != 1 {
			t.Fatalf("update was given %+v, want the saved record the second time", seen)
		}
		if got := mustRecords(t, repo, "ph-1")["ph-1"].FeedbackCount; got != 2 {
			t.Errorf("FeedbackCount = %d, want 2", got)
		}
	})

	t.Run("the same feedback is counted once", func(t *testing.T) {
		repo := newRepo(t)
		var seen []domain.Record
		mustApply(t, repo, fb("fb-1", "ph-1", t0), count(&seen), true)
		mustApply(t, repo, fb("fb-1", "ph-1", t0), count(&seen), false)
		if len(seen) != 1 {
			t.Errorf("update ran %d times, want once", len(seen))
		}
		if got := mustRecords(t, repo, "ph-1")["ph-1"].FeedbackCount; got != 1 {
			t.Errorf("FeedbackCount = %d, want 1", got)
		}
	})

	t.Run("records are kept per pharmacy and only requested ones are returned", func(t *testing.T) {
		repo := newRepo(t)
		mustApply(t, repo, fb("fb-1", "ph-1", t0), set(domain.Record{FeedbackCount: 1, LastAt: t0}), true)
		mustApply(t, repo, fb("fb-2", "ph-2", t0), set(domain.Record{FeedbackCount: 2, LastAt: t0}), true)
		mustApply(t, repo, fb("fb-3", "ph-3", t0), set(domain.Record{FeedbackCount: 3, LastAt: t0}), true)

		got := mustRecords(t, repo, "ph-3", "ph-1", "ph-unknown")
		if len(got) != 2 || got["ph-1"].FeedbackCount != 1 || got["ph-3"].FeedbackCount != 3 {
			t.Fatalf("Records = %+v", got)
		}
		if got["ph-1"].PharmacyID != "ph-1" || got["ph-3"].PharmacyID != "ph-3" {
			t.Errorf("records carry the wrong pharmacy IDs: %+v", got)
		}
	})

	t.Run("concurrent feedback for one pharmacy is never lost", func(t *testing.T) {
		repo := newRepo(t)
		const n = 20
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := repo.ApplyFeedback(ctx, fb(fmt.Sprintf("fb-%d", i), "ph-1", t0), func(r domain.Record) domain.Record {
					r.FeedbackCount++
					return r
				})
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if got := mustRecords(t, repo, "ph-1")["ph-1"].FeedbackCount; got != n {
			t.Errorf("FeedbackCount = %d after %d concurrent answers, want %d", got, n, n)
		}
	})

	t.Run("the same feedback delivered concurrently is counted once", func(t *testing.T) {
		repo := newRepo(t)
		const n = 10
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			applied int
		)
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ok, err := repo.ApplyFeedback(ctx, fb("fb-1", "ph-1", t0), func(r domain.Record) domain.Record {
					r.FeedbackCount++
					return r
				})
				if err != nil {
					t.Error(err)
				}
				mu.Lock()
				defer mu.Unlock()
				if ok {
					applied++
				}
			}()
		}
		wg.Wait()
		if applied != 1 {
			t.Errorf("applied %d times, want 1", applied)
		}
		if got := mustRecords(t, repo, "ph-1")["ph-1"].FeedbackCount; got != 1 {
			t.Errorf("FeedbackCount = %d, want 1", got)
		}
	})
}

func mustApply(t *testing.T, repo ports.TrustRepository, f domain.Feedback, update func(domain.Record) domain.Record, wantApplied bool) {
	t.Helper()
	applied, err := repo.ApplyFeedback(context.Background(), f, update)
	if err != nil {
		t.Fatal(err)
	}
	if applied != wantApplied {
		t.Fatalf("ApplyFeedback(%s) applied = %v, want %v", f.ID, applied, wantApplied)
	}
}

func mustRecords(t *testing.T, repo ports.TrustRepository, ids ...string) map[string]domain.Record {
	t.Helper()
	got, err := repo.Records(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// same compares records, with LastAt compared as an instant (Postgres returns
// it in the session time zone).
func same(a, b domain.Record) bool {
	at, bt := a.LastAt, b.LastAt
	a.LastAt, b.LastAt = time.Time{}, time.Time{}
	return a == b && at.Equal(bt)
}
