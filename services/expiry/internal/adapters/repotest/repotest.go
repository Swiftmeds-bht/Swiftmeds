// Package repotest holds the contract tests that every ports.DeadlineRepository
// must pass. Each adapter's test calls Run with a function that returns an
// empty repository.
package repotest

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/ports"
)

// Whole seconds, so databases that keep fewer decimals still pass.
var t0 = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

// Run calls every contract test with a new, empty repository.
func Run(t *testing.T, newRepo func(*testing.T) ports.DeadlineRepository) {
	t.Helper()
	tests := []struct {
		name string
		run  func(*testing.T, ports.DeadlineRepository)
	}{
		{"a saved deadline is listed once it is due", saveThenList},
		{"saving a hold again keeps its first deadline", saveKeepsTheFirstDeadline},
		{"a deadline exactly at as_of is due, one second later is not", boundary},
		{"due deadlines come oldest first, then by hold id", ordering},
		{"list pages through the due deadlines after a cursor", paging},
		{"a settled deadline is no longer listed", completeRemoves},
		{"a deadline is settled only once", completeOnce},
		{"settling an unknown hold changes nothing", completeUnknown},
		{"only one of many concurrent completes wins", concurrentComplete},
		{"a settled hold is not brought back by saving it again", saveAfterComplete},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newRepo(t)) })
	}
}

func save(t *testing.T, repo ports.DeadlineRepository, holdID string, expiresAt time.Time) domain.Deadline {
	t.Helper()
	d, err := domain.NewDeadline(holdID, expiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	return d
}

func listDue(t *testing.T, repo ports.DeadlineRepository, asOf time.Time) []string {
	t.Helper()
	due, err := repo.ListDue(context.Background(), asOf, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return ids(due)
}

func ids(deadlines []domain.Deadline) []string {
	out := make([]string, 0, len(deadlines))
	for _, d := range deadlines {
		out = append(out, d.HoldID)
	}
	return out
}

func saveThenList(t *testing.T, repo ports.DeadlineRepository) {
	want := save(t, repo, "h-1", t0)

	due, err := repo.ListDue(context.Background(), t0.Add(time.Minute), nil, 10)

	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].HoldID != want.HoldID || !due[0].ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("due = %+v, want [%+v]", due, want)
	}
}

func saveKeepsTheFirstDeadline(t *testing.T, repo ports.DeadlineRepository) {
	save(t, repo, "h-1", t0)
	save(t, repo, "h-1", t0.Add(time.Hour))

	due, err := repo.ListDue(context.Background(), t0.Add(24*time.Hour), nil, 10)

	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || !due[0].ExpiresAt.Equal(t0) {
		t.Fatalf("due = %+v, want one deadline at the first time %v", due, t0)
	}
}

// The ticket: "Boundary test: a hold expiring exactly now."
func boundary(t *testing.T, repo ports.DeadlineRepository) {
	save(t, repo, "h-1", t0)

	cases := []struct {
		name string
		asOf time.Time
		want []string
	}{
		{"one second before", t0.Add(-time.Second), []string{}},
		{"exactly at the deadline", t0, []string{"h-1"}},
		{"one second after", t0.Add(time.Second), []string{"h-1"}},
	}
	for _, c := range cases {
		if got := listDue(t, repo, c.asOf); !slices.Equal(got, c.want) {
			t.Errorf("%s: due = %v, want %v", c.name, got, c.want)
		}
	}
}

func ordering(t *testing.T, repo ports.DeadlineRepository) {
	save(t, repo, "h-c", t0.Add(2*time.Second))
	save(t, repo, "h-b", t0)
	save(t, repo, "h-a", t0)
	save(t, repo, "h-late", t0.Add(time.Hour))

	got := listDue(t, repo, t0.Add(time.Minute))

	if want := []string{"h-a", "h-b", "h-c"}; !slices.Equal(got, want) {
		t.Errorf("due = %v, want %v (h-late is not due yet)", got, want)
	}
}

func paging(t *testing.T, repo ports.DeadlineRepository) {
	ctx := context.Background()
	for i := range 5 {
		save(t, repo, fmt.Sprintf("h-%d", i), t0.Add(time.Duration(i)*time.Second))
	}
	asOf := t0.Add(time.Hour)

	var after *domain.Deadline
	var pages [][]string
	for range 4 {
		page, err := repo.ListDue(ctx, asOf, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, ids(page))
		if len(page) == 0 {
			break
		}
		last := page[len(page)-1]
		after = &last
	}

	want := [][]string{{"h-0", "h-1"}, {"h-2", "h-3"}, {"h-4"}, {}}
	if len(pages) != len(want) {
		t.Fatalf("pages = %v, want %v", pages, want)
	}
	for i := range want {
		if !slices.Equal(pages[i], want[i]) {
			t.Errorf("page %d = %v, want %v", i, pages[i], want[i])
		}
	}
}

func completeRemoves(t *testing.T, repo ports.DeadlineRepository) {
	save(t, repo, "h-1", t0)
	save(t, repo, "h-2", t0)

	won, err := repo.Complete(context.Background(), "h-1", domain.OutcomeExpired, t0.Add(time.Minute))

	if err != nil || !won {
		t.Fatalf("Complete = (%v, %v), want (true, nil)", won, err)
	}
	if got := listDue(t, repo, t0.Add(time.Hour)); !slices.Equal(got, []string{"h-2"}) {
		t.Errorf("due = %v, want only h-2", got)
	}
}

func completeOnce(t *testing.T, repo ports.DeadlineRepository) {
	ctx := context.Background()
	save(t, repo, "h-1", t0)

	first, err := repo.Complete(ctx, "h-1", domain.OutcomeExpired, t0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.Complete(ctx, "h-1", domain.OutcomeAlreadyFinal, t0)
	if err != nil {
		t.Fatal(err)
	}

	if !first || second {
		t.Errorf("first = %v, second = %v, want true then false", first, second)
	}
}

func completeUnknown(t *testing.T, repo ports.DeadlineRepository) {
	won, err := repo.Complete(context.Background(), "nope", domain.OutcomeExpired, t0)

	if err != nil || won {
		t.Fatalf("Complete = (%v, %v), want (false, nil)", won, err)
	}
}

func concurrentComplete(t *testing.T, repo ports.DeadlineRepository) {
	ctx := context.Background()
	save(t, repo, "h-1", t0)
	const callers = 30

	results := make(chan bool, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			won, err := repo.Complete(ctx, "h-1", domain.OutcomeExpired, t0)
			results <- won
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}
	wins := 0
	for won := range results {
		if won {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d callers won, want exactly 1", wins)
	}
}

func saveAfterComplete(t *testing.T, repo ports.DeadlineRepository) {
	save(t, repo, "h-1", t0)
	if _, err := repo.Complete(context.Background(), "h-1", domain.OutcomeExpired, t0); err != nil {
		t.Fatal(err)
	}

	save(t, repo, "h-1", t0)

	if got := listDue(t, repo, t0.Add(time.Hour)); len(got) != 0 {
		t.Errorf("due = %v, want none: a redelivered event must not reopen a settled deadline", got)
	}
}
