package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// seed stores holds directly in the repository (the limit does not apply) with
// created_at one minute apart: h-0 is the oldest.
func seed(t *testing.T, repo *memory.Repository, userID, pharmacyID string, count int) {
	t.Helper()
	for i := range count {
		h, err := domain.NewHold(fmt.Sprintf("h-%d", i), userID, pharmacyID, "med-1", 1, t0.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Create(context.Background(), h, 0); err != nil {
			t.Fatal(err)
		}
	}
}

func ids(holds []domain.Hold) []string {
	out := make([]string, 0, len(holds))
	for _, h := range holds {
		out = append(out, h.ID)
	}
	return out
}

func TestListHoldsPagesThroughAPatientsHolds(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newTestService(t)
	seed(t, repo, "u-1", "ph-1", 5)

	first, err := svc.ListHolds(ctx, ListQuery{UserID: "u-1", PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.ListHolds(ctx, ListQuery{UserID: "u-1", PageSize: 2, PageToken: first.NextPageToken})
	if err != nil {
		t.Fatal(err)
	}
	third, err := svc.ListHolds(ctx, ListQuery{UserID: "u-1", PageSize: 2, PageToken: second.NextPageToken})
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"h-4", "h-3"}; !slices.Equal(ids(first.Holds), want) || first.NextPageToken == "" {
		t.Errorf("first page = %v (next %q), want %v and a next page", ids(first.Holds), first.NextPageToken, want)
	}
	if want := []string{"h-2", "h-1"}; !slices.Equal(ids(second.Holds), want) || second.NextPageToken == "" {
		t.Errorf("second page = %v (next %q), want %v and a next page", ids(second.Holds), second.NextPageToken, want)
	}
	if want := []string{"h-0"}; !slices.Equal(ids(third.Holds), want) || third.NextPageToken != "" {
		t.Errorf("last page = %v (next %q), want %v and no next page", ids(third.Holds), third.NextPageToken, want)
	}
}

func TestListHoldsWithAnExactlyFullLastPageHasNoNextPage(t *testing.T) {
	svc, repo, _ := newTestService(t)
	seed(t, repo, "u-1", "ph-1", 4)

	got, err := svc.ListHolds(context.Background(), ListQuery{UserID: "u-1", PageSize: 4})

	if err != nil {
		t.Fatal(err)
	}
	if len(got.Holds) != 4 || got.NextPageToken != "" {
		t.Errorf("got %d holds and next %q, want 4 holds and no next page", len(got.Holds), got.NextPageToken)
	}
}

func TestListHoldsByPharmacyAndStatus(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newTestService(t)
	seed(t, repo, "u-1", "ph-1", 3)
	if _, err := svc.CancelHold(ctx, "h-1", "u-1", "changed my mind"); err != nil {
		t.Fatal(err)
	}

	all, err := svc.ListHolds(ctx, ListQuery{PharmacyID: "ph-1"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := svc.ListHolds(ctx, ListQuery{PharmacyID: "ph-1", Status: domain.Cancelled})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.ListHolds(ctx, ListQuery{PharmacyID: "ph-9"})
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"h-2", "h-1", "h-0"}; !slices.Equal(ids(all.Holds), want) {
		t.Errorf("all = %v, want %v", ids(all.Holds), want)
	}
	if want := []string{"h-1"}; !slices.Equal(ids(cancelled.Holds), want) {
		t.Errorf("cancelled = %v, want %v", ids(cancelled.Holds), want)
	}
	if len(other.Holds) != 0 {
		t.Errorf("another pharmacy = %v, want none", ids(other.Holds))
	}
}

func TestListHoldsNeedsExactlyOneOfPatientAndPharmacy(t *testing.T) {
	svc, _, _ := newTestService(t)

	for _, q := range []ListQuery{{}, {UserID: "u-1", PharmacyID: "ph-1"}} {
		if _, err := svc.ListHolds(context.Background(), q); !errors.Is(err, domain.ErrInvalidListFilter) {
			t.Errorf("query %+v: err = %v, want ErrInvalidListFilter", q, err)
		}
	}
}

func TestListHoldsRejectsAnUnknownStatusAndABadToken(t *testing.T) {
	svc, _, _ := newTestService(t)

	_, err := svc.ListHolds(context.Background(), ListQuery{UserID: "u-1", Status: "DONE"})
	if !errors.Is(err, domain.ErrInvalidListFilter) {
		t.Errorf("unknown status: err = %v, want ErrInvalidListFilter", err)
	}
	_, err = svc.ListHolds(context.Background(), ListQuery{UserID: "u-1", PageToken: "!!not-a-token"})
	if !errors.Is(err, domain.ErrInvalidPageToken) {
		t.Errorf("bad token: err = %v, want ErrInvalidPageToken", err)
	}
}

func TestListHoldsPageSizeDefaultsAndIsCapped(t *testing.T) {
	ctx := context.Background()
	repo := &hookRepo{HoldRepository: memory.NewRepository()}
	svc := NewService(repo, testConfig(), (&clock{t: t0}).now)

	cases := []struct {
		asked, wantLimit int
	}{
		{0, DefaultPageSize + 1},
		{-5, DefaultPageSize + 1},
		{7, 8},
		{MaxPageSize, MaxPageSize + 1},
		{10 * MaxPageSize, MaxPageSize + 1},
	}
	for _, c := range cases {
		if _, err := svc.ListHolds(ctx, ListQuery{UserID: "u-1", PageSize: c.asked}); err != nil {
			t.Fatal(err)
		}
		// The service asks for one extra hold to learn whether another page follows.
		if repo.lastList.Limit != c.wantLimit {
			t.Errorf("page size %d: repository limit = %d, want %d", c.asked, repo.lastList.Limit, c.wantLimit)
		}
	}
}

func TestListHoldsReportsARepositoryError(t *testing.T) {
	svc := NewService(errRepo{}, testConfig(), (&clock{t: t0}).now)

	if _, err := svc.ListHolds(context.Background(), ListQuery{UserID: "u-1"}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}
