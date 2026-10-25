// Package repotest is the contract test suite for ports.PriceRepository.
// Every adapter runs it, so the in-memory and Postgres repositories are
// guaranteed to behave the same way:
//
//	repotest.Run(t, func(t *testing.T) ports.PriceRepository { return memory.NewRepository() })
package repotest

import (
	"context"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/ports"
)

// Factory returns an empty repository for one subtest.
type Factory func(t *testing.T) ports.PriceRepository

var (
	t0 = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	t1 = t0.Add(time.Hour)
)

func p(pharmacy, product string, amount int64, at time.Time) domain.Price {
	return domain.Price{Key: domain.Key{PharmacyID: pharmacy, ProductID: product}, AmountChhertum: amount, UpdatedAt: at}
}

// Run executes every contract case against repositories made by newRepo.
func Run(t *testing.T, newRepo Factory) {
	t.Run("saved prices can be read back", func(t *testing.T) {
		repo := newRepo(t)
		want := []domain.Price{p("ph-1", "pr-1", 4500, t0), p("ph-2", "pr-1", 3900, t0)}
		mustSave(t, repo, want...)

		got := mustGet(t, repo, want[0].Key, want[1].Key)
		assertSameSet(t, got, want)
	})

	t.Run("saving an existing key replaces amount and time", func(t *testing.T) {
		repo := newRepo(t)
		mustSave(t, repo, p("ph-1", "pr-1", 4500, t0))
		mustSave(t, repo, p("ph-1", "pr-1", 4200, t1))

		got := mustGet(t, repo, domain.Key{PharmacyID: "ph-1", ProductID: "pr-1"})
		assertSameSet(t, got, []domain.Price{p("ph-1", "pr-1", 4200, t1)})
	})

	t.Run("missing keys are skipped", func(t *testing.T) {
		repo := newRepo(t)
		mustSave(t, repo, p("ph-1", "pr-1", 4500, t0))

		got := mustGet(t, repo,
			domain.Key{PharmacyID: "ph-1", ProductID: "pr-1"},
			domain.Key{PharmacyID: "ph-1", ProductID: "unknown"},
			domain.Key{PharmacyID: "unknown", ProductID: "pr-1"},
		)
		assertSameSet(t, got, []domain.Price{p("ph-1", "pr-1", 4500, t0)})
	})

	t.Run("a repeated key returns the price once", func(t *testing.T) {
		repo := newRepo(t)
		mustSave(t, repo, p("ph-1", "pr-1", 4500, t0))
		k := domain.Key{PharmacyID: "ph-1", ProductID: "pr-1"}

		if got := mustGet(t, repo, k, k, k); len(got) != 1 {
			t.Fatalf("got %d prices for one repeated key, want 1", len(got))
		}
	})

	t.Run("no keys returns no prices", func(t *testing.T) {
		repo := newRepo(t)
		mustSave(t, repo, p("ph-1", "pr-1", 4500, t0))
		if got := mustGet(t, repo); len(got) != 0 {
			t.Fatalf("got %v, want none", got)
		}
	})

	t.Run("saving nothing is not an error", func(t *testing.T) {
		mustSave(t, newRepo(t))
	})

	t.Run("prices are kept per pharmacy and product", func(t *testing.T) {
		repo := newRepo(t)
		mustSave(t, repo, p("ph-1", "pr-1", 100, t0), p("ph-1", "pr-2", 200, t0), p("ph-2", "pr-1", 300, t0))

		got := mustGet(t, repo, domain.Key{PharmacyID: "ph-1", ProductID: "pr-2"})
		assertSameSet(t, got, []domain.Price{p("ph-1", "pr-2", 200, t0)})
	})
}

func mustSave(t *testing.T, repo ports.PriceRepository, prices ...domain.Price) {
	t.Helper()
	if err := repo.SaveAll(context.Background(), prices); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}
}

func mustGet(t *testing.T, repo ports.PriceRepository, keys ...domain.Key) []domain.Price {
	t.Helper()
	got, err := repo.GetMany(context.Background(), keys)
	if err != nil {
		t.Fatalf("GetMany: %v", err)
	}
	return got
}

// assertSameSet compares ignoring order, and compares times with Equal so a
// database returning another time zone still matches.
func assertSameSet(t *testing.T, got, want []domain.Price) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d prices %v, want %d %v", len(got), got, len(want), want)
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g.Key == w.Key && g.AmountChhertum == w.AmountChhertum && g.UpdatedAt.Equal(w.UpdatedAt) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %+v in %+v", w, got)
		}
	}
}
