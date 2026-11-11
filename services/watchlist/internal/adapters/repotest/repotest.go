// Package repotest is the contract test suite for ports.SubscriptionRepository.
// Every adapter runs it, so the in-memory and Postgres repositories are
// guaranteed to behave the same way:
//
//	repotest.Run(t, func(t *testing.T) ports.SubscriptionRepository { return memory.NewRepository() })
package repotest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/ports"
)

// Factory returns an empty repository for one subtest.
type Factory func(t *testing.T) ports.SubscriptionRepository

var (
	t0      = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	thimphu = domain.Point{Lat: 27.4728, Lng: 89.639}
)

func sub(id, user, product string, at time.Time) domain.Subscription {
	return domain.Subscription{ID: id, UserID: user, ProductID: product, Location: thimphu, RadiusM: 5000, CreatedAt: at}
}

func alert(id string, s domain.Subscription, pharmacy string, at time.Time) domain.Alert {
	return domain.Alert{
		ID: id, SubscriptionID: s.ID, UserID: s.UserID, ProductID: s.ProductID,
		PharmacyID: pharmacy, QuantityAvailable: 4, OccurredAt: at,
	}
}

// Run executes every contract case against repositories made by newRepo.
func Run(t *testing.T, newRepo Factory) {
	ctx := context.Background()

	t.Run("a created subscription is listed for its user", func(t *testing.T) {
		repo := newRepo(t)
		s := sub("sub-1", "user-1", "pr-1", t0)
		got, created := mustCreate(t, repo, s)
		if !created || !same(got, s) {
			t.Fatalf("Create = %+v, %v; want %+v, true", got, created, s)
		}
		assertIDs(t, mustList(t, repo, "user-1"), "sub-1")
	})

	t.Run("a second active subscription for the same product returns the first", func(t *testing.T) {
		repo := newRepo(t)
		first := sub("sub-1", "user-1", "pr-1", t0)
		mustCreate(t, repo, first)

		got, created := mustCreate(t, repo, sub("sub-2", "user-1", "pr-1", t0.Add(time.Minute)))
		if created || got.ID != "sub-1" {
			t.Fatalf("Create duplicate = %s, created=%v; want sub-1, false", got.ID, created)
		}
		assertIDs(t, mustList(t, repo, "user-1"), "sub-1")
	})

	t.Run("other users and other products are separate subscriptions", func(t *testing.T) {
		repo := newRepo(t)
		mustCreate(t, repo, sub("sub-1", "user-1", "pr-1", t0))
		if _, created := mustCreate(t, repo, sub("sub-2", "user-2", "pr-1", t0)); !created {
			t.Error("another user's subscription for the same product was treated as a duplicate")
		}
		if _, created := mustCreate(t, repo, sub("sub-3", "user-1", "pr-2", t0)); !created {
			t.Error("the same user's subscription for another product was treated as a duplicate")
		}
	})

	t.Run("after an alert the user can subscribe to the same product again", func(t *testing.T) {
		repo := newRepo(t)
		first := sub("sub-1", "user-1", "pr-1", t0)
		mustCreate(t, repo, first)
		mustNotify(t, repo, alert("al-1", first, "ph-1", t0.Add(time.Hour)), true)

		if _, created := mustCreate(t, repo, sub("sub-2", "user-1", "pr-1", t0.Add(2*time.Hour))); !created {
			t.Fatal("a new subscription after a notified one was treated as a duplicate")
		}
	})

	t.Run("subscriptions are listed newest first, only for that user", func(t *testing.T) {
		repo := newRepo(t)
		mustCreate(t, repo, sub("old", "user-1", "pr-1", t0))
		mustCreate(t, repo, sub("new", "user-1", "pr-2", t0.Add(time.Hour)))
		mustCreate(t, repo, sub("other", "user-2", "pr-1", t0))

		assertIDs(t, mustList(t, repo, "user-1"), "new", "old")
		assertIDs(t, mustList(t, repo, "nobody"))
	})

	t.Run("delete removes only the owner's subscription", func(t *testing.T) {
		repo := newRepo(t)
		mustCreate(t, repo, sub("sub-1", "user-1", "pr-1", t0))

		if err := repo.Delete(ctx, "sub-1", "user-2"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Delete by another user: err = %v, want ErrNotFound", err)
		}
		if err := repo.Delete(ctx, "sub-1", "user-1"); err != nil {
			t.Fatalf("Delete by owner: %v", err)
		}
		assertIDs(t, mustList(t, repo, "user-1"))
		if err := repo.Delete(ctx, "sub-1", "user-1"); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Delete twice: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("active subscriptions for a product leave out notified ones", func(t *testing.T) {
		repo := newRepo(t)
		a := sub("a", "user-1", "pr-1", t0)
		b := sub("b", "user-2", "pr-1", t0)
		mustCreate(t, repo, a)
		mustCreate(t, repo, b)
		mustCreate(t, repo, sub("c", "user-1", "pr-2", t0))
		mustNotify(t, repo, alert("al-1", a, "ph-1", t0.Add(time.Hour)), true)

		got, err := repo.ActiveForProduct(ctx, "pr-1")
		if err != nil {
			t.Fatal(err)
		}
		assertIDs(t, got, "b")
		if !same(got[0], b) {
			t.Errorf("ActiveForProduct returned %+v, want %+v", got[0], b)
		}
	})

	t.Run("notify marks the subscription once", func(t *testing.T) {
		repo := newRepo(t)
		s := sub("sub-1", "user-1", "pr-1", t0)
		mustCreate(t, repo, s)
		at := t0.Add(time.Hour)

		mustNotify(t, repo, alert("al-1", s, "ph-1", at), true)
		mustNotify(t, repo, alert("al-2", s, "ph-2", at.Add(time.Minute)), false)

		list := mustList(t, repo, "user-1")
		if len(list) != 1 || !list[0].NotifiedAt.Equal(at) {
			t.Fatalf("after notify: %+v, want NotifiedAt %v", list, at)
		}
	})

	t.Run("notify for a missing subscription saves nothing", func(t *testing.T) {
		repo := newRepo(t)
		mustNotify(t, repo, alert("al-1", sub("gone", "user-1", "pr-1", t0), "ph-1", t0), false)
		assertAlertIDs(t, mustPending(t, repo, "user-1"))
	})

	t.Run("pending alerts are the user's undelivered ones, oldest first", func(t *testing.T) {
		repo := newRepo(t)
		a, b, other := sub("a", "user-1", "pr-1", t0), sub("b", "user-1", "pr-2", t0), sub("c", "user-2", "pr-1", t0)
		for _, s := range []domain.Subscription{a, b, other} {
			mustCreate(t, repo, s)
		}
		mustNotify(t, repo, alert("al-late", a, "ph-1", t0.Add(2*time.Hour)), true)
		mustNotify(t, repo, alert("al-early", b, "ph-1", t0.Add(time.Hour)), true)
		mustNotify(t, repo, alert("al-other", other, "ph-1", t0.Add(time.Hour)), true)

		got := mustPending(t, repo, "user-1")
		assertAlertIDs(t, got, "al-early", "al-late")
		want := alert("al-early", b, "ph-1", t0.Add(time.Hour))
		if g := got[0]; g.ID != want.ID || g.SubscriptionID != want.SubscriptionID || g.UserID != want.UserID ||
			g.ProductID != want.ProductID || g.PharmacyID != want.PharmacyID ||
			g.QuantityAvailable != want.QuantityAvailable || !g.OccurredAt.Equal(want.OccurredAt) {
			t.Errorf("PendingAlerts[0] = %+v, want %+v", g, want)
		}
	})

	t.Run("a delivered alert is no longer pending, and marking twice is fine", func(t *testing.T) {
		repo := newRepo(t)
		s := sub("sub-1", "user-1", "pr-1", t0)
		mustCreate(t, repo, s)
		mustNotify(t, repo, alert("al-1", s, "ph-1", t0.Add(time.Hour)), true)

		for range 2 {
			if err := repo.MarkDelivered(ctx, "al-1", t0.Add(2*time.Hour)); err != nil {
				t.Fatalf("MarkDelivered: %v", err)
			}
		}
		assertAlertIDs(t, mustPending(t, repo, "user-1"))
	})

	t.Run("marking an unknown alert is not an error", func(t *testing.T) {
		if err := newRepo(t).MarkDelivered(ctx, "nope", t0); err != nil {
			t.Fatalf("MarkDelivered(unknown): %v", err)
		}
	})
}

func mustPending(t *testing.T, repo ports.SubscriptionRepository, user string) []domain.Alert {
	t.Helper()
	got, err := repo.PendingAlerts(context.Background(), user)
	if err != nil {
		t.Fatalf("PendingAlerts: %v", err)
	}
	return got
}

func assertAlertIDs(t *testing.T, got []domain.Alert, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d alerts %+v, want %v", len(got), got, want)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Errorf("position %d: got %s, want %s", i, got[i].ID, want[i])
		}
	}
}

func mustCreate(t *testing.T, repo ports.SubscriptionRepository, s domain.Subscription) (domain.Subscription, bool) {
	t.Helper()
	got, created, err := repo.Create(context.Background(), s)
	if err != nil {
		t.Fatalf("Create(%s): %v", s.ID, err)
	}
	return got, created
}

func mustList(t *testing.T, repo ports.SubscriptionRepository, user string) []domain.Subscription {
	t.Helper()
	got, err := repo.ListByUser(context.Background(), user)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	return got
}

func mustNotify(t *testing.T, repo ports.SubscriptionRepository, a domain.Alert, want bool) {
	t.Helper()
	got, err := repo.Notify(context.Background(), a)
	if err != nil {
		t.Fatalf("Notify(%s): %v", a.ID, err)
	}
	if got != want {
		t.Fatalf("Notify(%s) = %v, want %v", a.ID, got, want)
	}
}

func assertIDs(t *testing.T, got []domain.Subscription, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d subscriptions %+v, want %v", len(got), got, want)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Errorf("position %d: got %s, want %s", i, got[i].ID, want[i])
		}
	}
}

// same compares with time.Equal, so a database returning another time zone still matches.
func same(a, b domain.Subscription) bool {
	return a.ID == b.ID && a.UserID == b.UserID && a.ProductID == b.ProductID &&
		a.Location == b.Location && a.RadiusM == b.RadiusM &&
		a.CreatedAt.Equal(b.CreatedAt) && a.NotifiedAt.Equal(b.NotifiedAt)
}
