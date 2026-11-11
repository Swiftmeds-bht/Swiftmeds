package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
)

var (
	now     = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	thimphu = domain.Point{Lat: 27.4728, Lng: 89.639}
	paro    = domain.Point{Lat: 27.4305, Lng: 89.4167}
)

// fakeGeo says a pharmacy is near a point if it is listed for that point.
type fakeGeo struct {
	near  map[domain.Point][]string
	err   error
	calls int
}

func (g *fakeGeo) WithinRadius(_ context.Context, pharmacyID string, center domain.Point, _ int32) (bool, error) {
	g.calls++
	if g.err != nil {
		return false, g.err
	}
	for _, id := range g.near[center] {
		if id == pharmacyID {
			return true, nil
		}
	}
	return false, nil
}

type fixture struct {
	svc  *Service
	repo *memory.Repository
	geo  *fakeGeo
}

func newFixture() fixture {
	repo := memory.NewRepository()
	geo := &fakeGeo{near: map[domain.Point][]string{thimphu: {"ph-thimphu"}, paro: {"ph-paro"}}}
	n := 0
	ids := func() string { n++; return fmt.Sprintf("id-%d", n) }
	return fixture{svc: NewService(repo, geo, func() time.Time { return now }, ids), repo: repo, geo: geo}
}

func (f fixture) subscribe(t *testing.T, user, product string, at domain.Point) domain.Subscription {
	t.Helper()
	s, err := f.svc.Subscribe(context.Background(), user, product, at, 5000)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	return s
}

func TestSubscribeCreatesWithGeneratedID(t *testing.T) {
	f := newFixture()
	s := f.subscribe(t, "user-1", "pr-1", thimphu)
	if s.ID != "id-1" || s.UserID != "user-1" || s.ProductID != "pr-1" || !s.CreatedAt.Equal(now) {
		t.Errorf("got %+v", s)
	}
}

func TestSubscribeTwiceReturnsTheSameSubscription(t *testing.T) {
	f := newFixture()
	first := f.subscribe(t, "user-1", "pr-1", thimphu)
	second := f.subscribe(t, "user-1", "pr-1", paro)
	if second.ID != first.ID {
		t.Errorf("duplicate subscribe returned %s, want %s", second.ID, first.ID)
	}
	list, _ := f.svc.List(context.Background(), "user-1")
	if len(list) != 1 {
		t.Errorf("user has %d subscriptions, want 1", len(list))
	}
}

func TestSubscribeRejectsInvalidInput(t *testing.T) {
	_, err := newFixture().svc.Subscribe(context.Background(), "user-1", "", thimphu, 5000)
	if !errors.Is(err, domain.ErrMissingProductID) {
		t.Errorf("err = %v, want ErrMissingProductID", err)
	}
}

func TestUnsubscribe(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	s := f.subscribe(t, "user-1", "pr-1", thimphu)

	if err := f.svc.Unsubscribe(ctx, s.ID, "user-2"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unsubscribe by another user: err = %v, want ErrNotFound", err)
	}
	if err := f.svc.Unsubscribe(ctx, s.ID, "user-1"); err != nil {
		t.Fatalf("unsubscribe by owner: %v", err)
	}
	if list, _ := f.svc.List(ctx, "user-1"); len(list) != 0 {
		t.Errorf("after unsubscribe: %+v", list)
	}
}

func TestUnsubscribeAndListNeedIDs(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	if err := f.svc.Unsubscribe(ctx, " ", "user-1"); !errors.Is(err, domain.ErrMissingSubscriptionID) {
		t.Errorf("no subscription id: err = %v", err)
	}
	if err := f.svc.Unsubscribe(ctx, "id-1", ""); !errors.Is(err, domain.ErrMissingUserID) {
		t.Errorf("no user id: err = %v", err)
	}
	if _, err := f.svc.List(ctx, ""); !errors.Is(err, domain.ErrMissingUserID) {
		t.Errorf("list without user id: err = %v", err)
	}
}

func increase(pharmacy, product string, available int32) domain.StockIncrease {
	return domain.StockIncrease{PharmacyID: pharmacy, ProductID: product, NewAvailable: available, OccurredAt: now.Add(time.Hour)}
}

func TestStockIncreaseAlertsOnlyNearbySubscribersOfThatProduct(t *testing.T) {
	f := newFixture()
	near := f.subscribe(t, "near", "pr-1", thimphu)
	f.subscribe(t, "far", "pr-1", paro)
	f.subscribe(t, "other-product", "pr-2", thimphu)

	alerts, err := f.svc.HandleStockIncreased(context.Background(), increase("ph-thimphu", "pr-1", 4))
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("got %d alerts %+v, want 1", len(alerts), alerts)
	}
	want := domain.Alert{
		ID: alerts[0].ID, SubscriptionID: near.ID, UserID: "near", ProductID: "pr-1",
		PharmacyID: "ph-thimphu", QuantityAvailable: 4, OccurredAt: now.Add(time.Hour),
	}
	if alerts[0] != want || alerts[0].ID == "" {
		t.Errorf("alert = %+v, want %+v", alerts[0], want)
	}
	if saved := f.repo.Alerts(); len(saved) != 1 || saved[0] != alerts[0] {
		t.Errorf("saved alerts = %+v", saved)
	}
}

func TestStockIncreaseAlertsEachSubscriptionOnce(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.subscribe(t, "user-1", "pr-1", thimphu)

	if alerts, _ := f.svc.HandleStockIncreased(ctx, increase("ph-thimphu", "pr-1", 4)); len(alerts) != 1 {
		t.Fatalf("first event: %d alerts, want 1", len(alerts))
	}
	alerts, err := f.svc.HandleStockIncreased(ctx, increase("ph-thimphu", "pr-1", 9))
	if err != nil || len(alerts) != 0 {
		t.Fatalf("second event: %d alerts, err %v; want none", len(alerts), err)
	}
}

func TestStockIncreaseToZeroDoesNothing(t *testing.T) {
	f := newFixture()
	f.subscribe(t, "user-1", "pr-1", thimphu)
	alerts, err := f.svc.HandleStockIncreased(context.Background(), increase("ph-thimphu", "pr-1", 0))
	if err != nil || len(alerts) != 0 || f.geo.calls != 0 {
		t.Errorf("alerts %v, err %v, geo calls %d; want none", alerts, err, f.geo.calls)
	}
}

func TestStockIncreaseWithoutSubscribersSkipsGeo(t *testing.T) {
	f := newFixture()
	if _, err := f.svc.HandleStockIncreased(context.Background(), increase("ph-thimphu", "pr-1", 4)); err != nil {
		t.Fatal(err)
	}
	if f.geo.calls != 0 {
		t.Errorf("geo called %d times with no subscribers", f.geo.calls)
	}
}

func TestStockIncreaseRejectsMalformedEvent(t *testing.T) {
	_, err := newFixture().svc.HandleStockIncreased(context.Background(), increase("", "pr-1", 4))
	if !errors.Is(err, domain.ErrMissingPharmacyID) {
		t.Errorf("err = %v, want ErrMissingPharmacyID", err)
	}
}

// If Geo fails, the event is retried later; nothing must be half-recorded.
func TestStockIncreaseGeoFailureReturnsErrorAndRecordsNothing(t *testing.T) {
	f := newFixture()
	f.subscribe(t, "user-1", "pr-1", thimphu)
	f.geo.err = errors.New("geo unavailable")

	if _, err := f.svc.HandleStockIncreased(context.Background(), increase("ph-thimphu", "pr-1", 4)); err == nil {
		t.Fatal("want an error when Geo fails")
	}
	if got := f.repo.Alerts(); len(got) != 0 {
		t.Errorf("alerts saved despite the failure: %+v", got)
	}
}

func TestNewIDIsAUUID(t *testing.T) {
	a, b := NewID(), NewID()
	if len(a) != 36 || a[14] != '4' || a == b {
		t.Errorf("NewID() = %q, %q; want two different version-4 UUIDs", a, b)
	}
}
