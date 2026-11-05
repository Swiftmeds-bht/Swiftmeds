// Package repotest is the contract test suite for ports.LocationRepository,
// run by every adapter so memory and Postgres behave the same way.
package repotest

import (
	"context"
	"sort"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/ports"
)

// Factory returns an empty repository for one subtest.
type Factory func(t *testing.T) ports.LocationRepository

var box = domain.Box{MinLat: 27.0, MaxLat: 28.0, MinLng: 89.0, MaxLng: 90.0}

func loc(id string, lat, lng float64) domain.Location {
	return domain.Location{PharmacyID: id, Point: domain.Point{Lat: lat, Lng: lng}}
}

func Run(t *testing.T, newRepo Factory) {
	t.Run("only locations inside the box are returned, edges included", func(t *testing.T) {
		repo := newRepo(t)
		mustUpsert(t, repo,
			loc("inside", 27.5, 89.5),
			loc("on-min-corner", 27.0, 89.0),
			loc("on-max-corner", 28.0, 90.0),
			loc("north", 28.0001, 89.5),
			loc("east", 27.5, 90.0001),
			loc("south", 26.9999, 89.5),
			loc("west", 27.5, 88.9999),
		)
		assertIDs(t, mustWithin(t, repo, box), "inside", "on-max-corner", "on-min-corner")
	})

	t.Run("upsert moves an existing pharmacy", func(t *testing.T) {
		repo := newRepo(t)
		mustUpsert(t, repo, loc("ph-1", 10, 10))
		mustUpsert(t, repo, loc("ph-1", 27.5, 89.5))

		got := mustWithin(t, repo, box)
		assertIDs(t, got, "ph-1")
		if got[0].Point != (domain.Point{Lat: 27.5, Lng: 89.5}) {
			t.Errorf("point = %+v, want the new location", got[0].Point)
		}
	})

	t.Run("remove hides a pharmacy and is idempotent", func(t *testing.T) {
		repo := newRepo(t)
		mustUpsert(t, repo, loc("ph-1", 27.5, 89.5), loc("ph-2", 27.6, 89.6))
		for i := 0; i < 2; i++ {
			if err := repo.Remove(context.Background(), "ph-1"); err != nil {
				t.Fatalf("remove #%d: %v", i+1, err)
			}
		}
		if err := repo.Remove(context.Background(), "never-existed"); err != nil {
			t.Fatalf("remove unknown: %v", err)
		}
		assertIDs(t, mustWithin(t, repo, box), "ph-2")
	})

	t.Run("empty repository returns nothing", func(t *testing.T) {
		if got := mustWithin(t, newRepo(t), box); len(got) != 0 {
			t.Fatalf("got %v, want none", got)
		}
	})
}

func mustUpsert(t *testing.T, repo ports.LocationRepository, locs ...domain.Location) {
	t.Helper()
	for _, l := range locs {
		if err := repo.Upsert(context.Background(), l); err != nil {
			t.Fatalf("Upsert(%v): %v", l, err)
		}
	}
}

func mustWithin(t *testing.T, repo ports.LocationRepository, b domain.Box) []domain.Location {
	t.Helper()
	got, err := repo.WithinBox(context.Background(), b)
	if err != nil {
		t.Fatalf("WithinBox: %v", err)
	}
	return got
}

func assertIDs(t *testing.T, got []domain.Location, want ...string) {
	t.Helper()
	ids := make([]string, 0, len(got))
	for _, l := range got {
		ids = append(ids, l.PharmacyID)
	}
	sort.Strings(ids)
	sort.Strings(want)
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range ids {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
}
