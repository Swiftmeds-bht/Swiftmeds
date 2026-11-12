package app

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/ports"
)

// ---------------------------------------------------------------------------
// Fakes. Every fake honours ctx (like real gRPC clients do), can be slowed down
// with delay and made to fail with err, and counts its calls.

type behaviour struct {
	delay time.Duration
	err   error
	calls atomic.Int32
}

// wait simulates network latency; it returns early with ctx.Err() on timeout.
func (b *behaviour) wait(ctx context.Context) error {
	b.calls.Add(1)
	if b.delay > 0 {
		select {
		case <-time.After(b.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return b.err
}

type fakeCatalog struct {
	behaviour
	products []domain.Product
}

func (f *fakeCatalog) ResolveName(ctx context.Context, q string, limit int) ([]domain.Product, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	var out []domain.Product
	for _, p := range f.products {
		if strings.Contains(strings.ToLower(p.BrandName+" "+p.GenericName), strings.ToLower(q)) {
			out = append(out, p)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type fakeGeo struct {
	behaviour
	nearby []ports.NearbyPharmacy
}

func (f *fakeGeo) FindNearby(ctx context.Context, _ domain.Point, radius int32, _ int) ([]ports.NearbyPharmacy, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	var out []ports.NearbyPharmacy
	for _, n := range f.nearby {
		if n.DistanceM <= radius {
			out = append(out, n)
		}
	}
	return out, nil
}

type fakeInventory struct {
	behaviour
	stock []ports.Stock
}

func (f *fakeInventory) AvailableStock(ctx context.Context, phs, prs []string) ([]ports.Stock, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	var out []ports.Stock
	for _, s := range f.stock {
		if contains(phs, s.PharmacyID) && contains(prs, s.ProductID) {
			out = append(out, s)
		}
	}
	return out, nil
}

type fakeFreshness struct {
	behaviour
	scores map[domain.Key]float64
}

func (f *fakeFreshness) Scores(ctx context.Context, _ []domain.Key) (map[domain.Key]float64, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	return f.scores, nil
}

type fakePricing struct {
	behaviour
	prices map[domain.Key]int64
}

func (f *fakePricing) Prices(ctx context.Context, _ []domain.Key) (map[domain.Key]int64, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	return f.prices, nil
}

type fakeTrust struct {
	behaviour
	scores map[string]float64
}

func (f *fakeTrust) Scores(ctx context.Context, _ []string) (map[string]float64, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	return f.scores, nil
}

type fakeRegistry struct {
	behaviour
	info map[string]ports.PharmacyInfo
}

func (f *fakeRegistry) Pharmacies(ctx context.Context, _ []string) (map[string]ports.PharmacyInfo, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	return f.info, nil
}

// fakeRanking orders by trust (desc) so tests can tell its order apart from
// the nearest-first fallback.
type fakeRanking struct {
	behaviour
	got []ports.RankCandidate
}

func (f *fakeRanking) Rank(ctx context.Context, cs []ports.RankCandidate) ([]ports.Ranked, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	f.got = cs
	sorted := append([]ports.RankCandidate(nil), cs...)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Trust > sorted[i].Trust {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	out := make([]ports.Ranked, len(sorted))
	for i, c := range sorted {
		out[i] = ports.Ranked{Key: c.Key, Score: c.Trust}
	}
	return out, nil
}

type fakeSubstitution struct {
	behaviour
	subs map[string][]domain.Product
}

func (f *fakeSubstitution) Substitutes(ctx context.Context, id string, _ int) ([]domain.Product, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	return f.subs[id], nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// A small world: Napa (paracetamol) in stock at ph-near and ph-far, Brufen
// (ibuprofen) nowhere, Profen (same molecule as Brufen) at ph-mid.

var (
	napa   = domain.Product{ID: "pr-napa", BrandName: "Napa", GenericName: "Paracetamol", Strength: "500mg"}
	brufen = domain.Product{ID: "pr-brufen", BrandName: "Brufen", GenericName: "Ibuprofen", Strength: "400mg"}
	profen = domain.Product{ID: "pr-profen", BrandName: "Profen", GenericName: "Ibuprofen", Strength: "400mg"}
	now    = time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
)

func k(ph, pr string) domain.Key { return domain.Key{PharmacyID: ph, ProductID: pr} }

type world struct {
	catalog  *fakeCatalog
	geo      *fakeGeo
	inv      *fakeInventory
	fresh    *fakeFreshness
	pricing  *fakePricing
	trust    *fakeTrust
	registry *fakeRegistry
	ranking  *fakeRanking
	subs     *fakeSubstitution
}

func newWorld() *world {
	return &world{
		catalog: &fakeCatalog{products: []domain.Product{napa, brufen, profen}},
		geo: &fakeGeo{nearby: []ports.NearbyPharmacy{
			{PharmacyID: "ph-near", DistanceM: 300}, {PharmacyID: "ph-mid", DistanceM: 1200}, {PharmacyID: "ph-far", DistanceM: 4000},
		}},
		inv: &fakeInventory{stock: []ports.Stock{
			{Key: k("ph-near", napa.ID), Available: 5, ConfirmedAt: now},
			{Key: k("ph-far", napa.ID), Available: 20, ConfirmedAt: now},
			{Key: k("ph-mid", napa.ID), Available: 0}, // out of stock
			{Key: k("ph-mid", profen.ID), Available: 3, ConfirmedAt: now},
			{Key: k("ph-elsewhere", napa.ID), Available: 9}, // not nearby
		}},
		fresh:   &fakeFreshness{scores: map[domain.Key]float64{k("ph-near", napa.ID): 0.9, k("ph-far", napa.ID): 0.4}},
		pricing: &fakePricing{prices: map[domain.Key]int64{k("ph-near", napa.ID): 1500, k("ph-far", napa.ID): 1200}},
		trust:   &fakeTrust{scores: map[string]float64{"ph-near": 0.6, "ph-far": 0.9, "ph-mid": 0.7}},
		registry: &fakeRegistry{info: map[string]ports.PharmacyInfo{
			"ph-near": {Name: "Near Pharmacy", Address: "Norzin Lam", Location: &domain.Point{Lat: 27.47, Lng: 89.63}},
			"ph-far":  {Name: "Far Pharmacy", Address: "Babesa"},
			"ph-mid":  {Name: "Mid Pharmacy", Address: "Changzamtog"},
		}},
		ranking: &fakeRanking{},
		subs:    &fakeSubstitution{subs: map[string][]domain.Product{brufen.ID: {profen, brufen}}},
	}
}

func (w *world) service(opts Options) *Service {
	if opts.NewID == nil {
		opts.NewID = func() string { return "search-1" }
	}
	return NewService(Deps{
		Catalog: w.catalog, Geo: w.geo, Inventory: w.inv, Freshness: w.fresh, Pricing: w.pricing,
		Trust: w.trust, Registry: w.registry, Ranking: w.ranking, Substitution: w.subs,
	}, opts)
}

func query(t *testing.T, text string) domain.Query {
	t.Helper()
	q, err := domain.NewQuery(text, domain.Point{Lat: 27.4728, Lng: 89.639}, 5000, 0)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func pharmacies(offers []domain.Offer) []string {
	out := make([]string, len(offers))
	for i, o := range offers {
		out[i] = o.PharmacyID
	}
	return out
}

// ---------------------------------------------------------------------------

func TestSearchHappyPath(t *testing.T) {
	w := newWorld()
	res, err := w.service(Options{}).Search(context.Background(), query(t, "napa"))
	if err != nil {
		t.Fatal(err)
	}
	if res.SearchID != "search-1" || len(res.Degraded) != 0 || len(res.Substitutes) != 0 {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Results) != 1 || res.Results[0].ID != napa.ID {
		t.Fatalf("results = %+v", res.Results)
	}
	offers := res.Results[0].Offers
	// Ranking order (fake ranks by trust): far (0.9) before near (0.6).
	// ph-mid has 0 units and ph-elsewhere is not nearby: both excluded.
	if got := pharmacies(offers); !reflect.DeepEqual(got, []string{"ph-far", "ph-near"}) {
		t.Fatalf("offers = %v", got)
	}
	near := offers[1]
	want := domain.Offer{
		PharmacyID: "ph-near", PharmacyName: "Near Pharmacy", Address: "Norzin Lam", Location: &domain.Point{Lat: 27.47, Lng: 89.63},
		DistanceM: 300, Quantity: 5, PriceChhertum: 1500, Freshness: 0.9, Trust: 0.6, RankScore: 0.6, ConfirmedAt: now,
	}
	if !reflect.DeepEqual(near, want) {
		t.Errorf("offer = %+v\nwant  %+v", near, want)
	}
	// Ranking received the enriched candidates.
	if len(w.ranking.got) != 2 {
		t.Errorf("ranking got %d candidates, want 2", len(w.ranking.got))
	}
}

// Ranking gets each pharmacy's score from the Trust service, not the neutral
// placeholder (SCRUM-73). Neutral is only for a pharmacy Trust did not return.
func TestRankingUsesTheTrustServiceScores(t *testing.T) {
	w := newWorld()
	w.trust.scores = map[string]float64{"ph-near": 0.0, "ph-far": 1.0} // the boundaries
	res, err := w.service(Options{}).Search(context.Background(), query(t, "napa"))
	if err != nil {
		t.Fatal(err)
	}
	if contains(res.Degraded, DepTrust) {
		t.Fatalf("degraded = %v, want trust answered", res.Degraded)
	}
	got := map[string]float64{}
	for _, c := range w.ranking.got {
		got[c.PharmacyID] = c.Trust
	}
	if want := map[string]float64{"ph-near": 0.0, "ph-far": 1.0}; !reflect.DeepEqual(got, want) {
		t.Errorf("trust given to Ranking = %v, want %v", got, want)
	}

	w.trust.scores = map[string]float64{"ph-far": 0.9} // ph-near missing from the answer
	if _, err := w.service(Options{}).Search(context.Background(), query(t, "napa")); err != nil {
		t.Fatal(err)
	}
	for _, c := range w.ranking.got {
		if c.PharmacyID == "ph-near" && c.Trust != NeutralTrust {
			t.Errorf("ph-near trust = %v, want neutral %v when Trust returns no score", c.Trust, NeutralTrust)
		}
	}
}

func TestSearchNoProductMatches(t *testing.T) {
	w := newWorld()
	res, err := w.service(Options{}).Search(context.Background(), query(t, "nonexistium"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 0 || w.inv.calls.Load() != 0 {
		t.Errorf("results = %+v, inventory calls = %d; want none", res.Results, w.inv.calls.Load())
	}
}

func TestSearchNoPharmaciesNearby(t *testing.T) {
	w := newWorld()
	w.geo.nearby = nil
	res, err := w.service(Options{}).Search(context.Background(), query(t, "napa"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 1 || len(res.Results[0].Offers) != 0 || w.inv.calls.Load() != 0 {
		t.Errorf("results = %+v; want napa with no offers and no inventory call", res.Results)
	}
}

func TestSearchCapsOffersPerProduct(t *testing.T) {
	w := newWorld()
	q := query(t, "napa")
	q.MaxOffers = 1
	res, err := w.service(Options{}).Search(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if got := pharmacies(res.Results[0].Offers); !reflect.DeepEqual(got, []string{"ph-far"}) {
		t.Errorf("offers = %v, want only the best one", got)
	}
}

func TestSearchCapsProducts(t *testing.T) {
	w := newWorld()
	w.catalog.products = nil
	for i := 0; i < 8; i++ {
		w.catalog.products = append(w.catalog.products, domain.Product{ID: string(rune('a' + i)), BrandName: "Napa"})
	}
	w.catalog.products = append(w.catalog.products, w.catalog.products...) // a catalog that ignores the limit
	res, err := w.service(Options{}).Search(context.Background(), query(t, "napa"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != MaxProducts {
		t.Errorf("%d results, want at most %d", len(res.Results), MaxProducts)
	}
}

// --- Substitutes ------------------------------------------------------------

func TestSearchSuggestsSubstitutesWhenNothingInStock(t *testing.T) {
	w := newWorld()
	res, err := w.service(Options{}).Search(context.Background(), query(t, "brufen"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 1 || len(res.Results[0].Offers) != 0 {
		t.Fatalf("results = %+v; want Brufen with no offers", res.Results)
	}
	// Substitution also returned Brufen itself: it must not be suggested back.
	if len(res.Substitutes) != 1 || res.Substitutes[0].ID != profen.ID {
		t.Fatalf("substitutes = %+v; want only Profen", res.Substitutes)
	}
	if got := pharmacies(res.Substitutes[0].Offers); !reflect.DeepEqual(got, []string{"ph-mid"}) {
		t.Errorf("substitute offers = %v", got)
	}
}

func TestSearchNoSubstitutesWhenSomethingIsInStock(t *testing.T) {
	w := newWorld()
	if _, err := w.service(Options{}).Search(context.Background(), query(t, "napa")); err != nil {
		t.Fatal(err)
	}
	if w.subs.calls.Load() != 0 {
		t.Errorf("Substitution called %d times, want 0", w.subs.calls.Load())
	}
}

func TestSearchSubstitutesNotInStockAreLeftOut(t *testing.T) {
	w := newWorld()
	w.inv.stock = w.inv.stock[:2] // only Napa stock; Profen nowhere
	res, err := w.service(Options{}).Search(context.Background(), query(t, "brufen"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Substitutes) != 0 {
		t.Errorf("substitutes = %+v; want none (none in stock)", res.Substitutes)
	}
}

func TestSearchSubstitutionDownIsPartial(t *testing.T) {
	w := newWorld()
	w.subs.err = errors.New("unavailable")
	res, err := w.service(Options{}).Search(context.Background(), query(t, "brufen"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Degraded, []string{DepSubstitution}) || len(res.Substitutes) != 0 {
		t.Errorf("degraded = %v, substitutes = %v", res.Degraded, res.Substitutes)
	}
}

func TestSearchWithoutSubstitutionServiceConfigured(t *testing.T) {
	w := newWorld()
	svc := w.service(Options{})
	svc.deps.Substitution = nil
	res, err := svc.Search(context.Background(), query(t, "brufen"))
	if err != nil || len(res.Substitutes) != 0 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

// --- Required services fail the search --------------------------------------

func TestFailuresLogTheRealCause(t *testing.T) {
	var buf strings.Builder
	w := newWorld()
	w.geo.err = errors.New("geo: connection refused")
	svc := w.service(Options{Logger: slog.New(slog.NewTextHandler(&buf, nil))})

	if _, err := svc.Search(context.Background(), query(t, "napa")); err == nil {
		t.Fatal("want an error")
	}
	sq, _ := domain.NewSuggestQuery("napa", 0)
	w.catalog.err = errors.New("catalog: no route to host")
	if _, err := svc.Autocomplete(context.Background(), sq); err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"search failed", "geo: connection refused", "autocomplete failed", "catalog: no route to host"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log does not contain %q:\n%s", want, buf.String())
		}
	}
}

func TestSearchFailsWhenARequiredServiceFails(t *testing.T) {
	boom := errors.New("connection refused")
	cases := map[string]func(*world){
		"catalog":   func(w *world) { w.catalog.err = boom },
		"geo":       func(w *world) { w.geo.err = boom },
		"inventory": func(w *world) { w.inv.err = boom },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			w := newWorld()
			breakIt(w)
			_, err := w.service(Options{}).Search(context.Background(), query(t, "napa"))
			if !errors.Is(err, ErrUpstream) || !errors.Is(err, boom) || !strings.Contains(err.Error(), name) {
				t.Fatalf("err = %v; want ErrUpstream naming %s", err, name)
			}
		})
	}
}

func TestSearchFailsWhenInventoryFailsForSubstitutes(t *testing.T) {
	w := newWorld()
	calls := 0
	w.inv.stock = nil // first call: nothing in stock -> substitutes are looked up
	inv := &failSecondInventory{inner: w.inv, calls: &calls}
	svc := w.service(Options{})
	svc.deps.Inventory = inv
	if _, err := svc.Search(context.Background(), query(t, "brufen")); !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

type failSecondInventory struct {
	inner *fakeInventory
	calls *int
}

func (f *failSecondInventory) AvailableStock(ctx context.Context, phs, prs []string) ([]ports.Stock, error) {
	*f.calls++
	if *f.calls > 1 {
		return nil, errors.New("inventory went down")
	}
	return f.inner.AvailableStock(ctx, phs, prs)
}

// --- Optional services degrade gracefully ------------------------------------

func TestSearchPartialResultsWhenOptionalServicesFail(t *testing.T) {
	boom := errors.New("unavailable")
	cases := []struct {
		dep    string
		break_ func(*world)
		check  func(t *testing.T, offers []domain.Offer)
	}{
		{DepFreshness, func(w *world) { w.fresh.err = boom }, func(t *testing.T, os []domain.Offer) {
			for _, o := range os {
				if o.Freshness != 0 {
					t.Errorf("%s freshness = %v, want 0 (unknown)", o.PharmacyID, o.Freshness)
				}
			}
		}},
		{DepPricing, func(w *world) { w.pricing.err = boom }, func(t *testing.T, os []domain.Offer) {
			for _, o := range os {
				if o.PriceChhertum != 0 {
					t.Errorf("%s price = %v, want 0 (unknown)", o.PharmacyID, o.PriceChhertum)
				}
			}
		}},
		{DepTrust, func(w *world) { w.trust.err = boom }, func(t *testing.T, os []domain.Offer) {
			for _, o := range os {
				if o.Trust != NeutralTrust {
					t.Errorf("%s trust = %v, want neutral %v", o.PharmacyID, o.Trust, NeutralTrust)
				}
			}
		}},
		{DepRegistry, func(w *world) { w.registry.err = boom }, func(t *testing.T, os []domain.Offer) {
			for _, o := range os {
				if o.PharmacyName != "" || o.Location != nil {
					t.Errorf("%s has registry details without registry", o.PharmacyID)
				}
			}
		}},
		{DepRanking, func(w *world) { w.ranking.err = boom }, func(t *testing.T, os []domain.Offer) {
			// Fallback: nearest first (near 300 m before far 4000 m), not trust order.
			if got := pharmacies(os); !reflect.DeepEqual(got, []string{"ph-near", "ph-far"}) {
				t.Errorf("fallback order = %v, want nearest first", got)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.dep, func(t *testing.T) {
			w := newWorld()
			tc.break_(w)
			res, err := w.service(Options{}).Search(context.Background(), query(t, "napa"))
			if err != nil {
				t.Fatalf("an optional service failing must not fail the search: %v", err)
			}
			if !reflect.DeepEqual(res.Degraded, []string{tc.dep}) {
				t.Errorf("degraded = %v, want [%s]", res.Degraded, tc.dep)
			}
			if len(res.Results) != 1 || len(res.Results[0].Offers) != 2 {
				t.Fatalf("results = %+v; want napa with 2 offers", res.Results)
			}
			tc.check(t, res.Results[0].Offers)
		})
	}
}

func TestSearchManyOptionalFailuresAreAllListedSorted(t *testing.T) {
	w := newWorld()
	boom := errors.New("down")
	w.trust.err, w.fresh.err, w.pricing.err = boom, boom, boom
	res, err := w.service(Options{}).Search(context.Background(), query(t, "napa"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{DepFreshness, DepPricing, DepTrust}; !reflect.DeepEqual(res.Degraded, want) {
		t.Errorf("degraded = %v, want %v", res.Degraded, want)
	}
}

// --- Timeouts and concurrency --------------------------------------------------

// The ticket's timeout case: Freshness hangs; the search still answers, on
// time, without freshness.
func TestSearchTimeoutOnOptionalServiceGivesPartialResults(t *testing.T) {
	w := newWorld()
	w.fresh.delay = time.Hour
	svc := w.service(Options{CallTimeout: 50 * time.Millisecond})

	start := time.Now()
	res, err := svc.Search(context.Background(), query(t, "napa"))
	took := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Degraded, []string{DepFreshness}) || len(res.Results[0].Offers) != 2 {
		t.Errorf("degraded = %v, offers = %d", res.Degraded, len(res.Results[0].Offers))
	}
	if took > 500*time.Millisecond {
		t.Errorf("search took %v; the per-call timeout should have cut Freshness off at ~50ms", took)
	}
}

func TestSearchTimeoutOnRequiredServiceFails(t *testing.T) {
	w := newWorld()
	w.geo.delay = time.Hour
	_, err := w.service(Options{CallTimeout: 50 * time.Millisecond}).Search(context.Background(), query(t, "napa"))
	if !errors.Is(err, ErrUpstream) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v; want ErrUpstream wrapping DeadlineExceeded", err)
	}
}

// With every dependency taking 100 ms, a sequential search would take about
// 100 (catalog) + 100 (geo) + 100 (inventory) + 4×100 (enrichment) + 100
// (ranking) = 800 ms. Concurrent calls bring it to about 400 ms.
func TestSearchCallsServicesConcurrently(t *testing.T) {
	w := newWorld()
	const d = 100 * time.Millisecond
	for _, b := range []*behaviour{&w.catalog.behaviour, &w.geo.behaviour, &w.inv.behaviour, &w.fresh.behaviour,
		&w.pricing.behaviour, &w.trust.behaviour, &w.registry.behaviour, &w.ranking.behaviour} {
		b.delay = d
	}
	start := time.Now()
	if _, err := w.service(Options{}).Search(context.Background(), query(t, "napa")); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	if took >= 7*d {
		t.Errorf("search took %v; calls are not running concurrently (sequential ≈ %v)", took, 8*d)
	}
}

// The 2 s budget holds even when every optional service hangs.
func TestSearchRespectsTheRequestBudget(t *testing.T) {
	w := newWorld()
	for _, b := range []*behaviour{&w.fresh.behaviour, &w.pricing.behaviour, &w.trust.behaviour, &w.registry.behaviour, &w.ranking.behaviour} {
		b.delay = time.Hour
	}
	svc := w.service(Options{CallTimeout: time.Hour, RequestTimeout: 200 * time.Millisecond})
	start := time.Now()
	res, err := svc.Search(context.Background(), query(t, "napa"))
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("search took %v, want about the 200ms budget", took)
	}
	if len(res.Degraded) != 5 {
		t.Errorf("degraded = %v, want all five optional services", res.Degraded)
	}
}

// Realistic latencies on the seed-sized data: well under the 2 s target.
func TestSearchLatencyWithRealisticDelaysIsUnderTwoSeconds(t *testing.T) {
	w := newWorld()
	delays := map[*behaviour]time.Duration{
		&w.catalog.behaviour: 40 * time.Millisecond, &w.geo.behaviour: 30 * time.Millisecond,
		&w.inv.behaviour: 50 * time.Millisecond, &w.fresh.behaviour: 30 * time.Millisecond,
		&w.pricing.behaviour: 30 * time.Millisecond, &w.trust.behaviour: 30 * time.Millisecond,
		&w.registry.behaviour: 40 * time.Millisecond, &w.ranking.behaviour: 10 * time.Millisecond,
	}
	for b, d := range delays {
		b.delay = d
	}
	start := time.Now()
	if _, err := w.service(Options{}).Search(context.Background(), query(t, "napa")); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took >= 2*time.Second {
		t.Errorf("search took %v, want < 2s", took)
	}
}

// --- Autocomplete --------------------------------------------------------------

func TestAutocomplete(t *testing.T) {
	w := newWorld()
	q, _ := domain.NewSuggestQuery("ibu", 1)
	got, err := w.service(Options{}).Autocomplete(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != brufen.ID {
		t.Errorf("suggestions = %+v, want only Brufen (limit 1)", got)
	}
}

func TestAutocompleteCapsACatalogThatIgnoresTheLimit(t *testing.T) {
	w := newWorld()
	q, _ := domain.NewSuggestQuery("ibu", 1)
	svc := w.service(Options{})
	svc.deps.Catalog = ignoreLimitCatalog{w.catalog.products}
	got, err := svc.Autocomplete(context.Background(), q)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %d suggestions (%v), want 1", len(got), err)
	}
}

type ignoreLimitCatalog struct{ products []domain.Product }

func (c ignoreLimitCatalog) ResolveName(context.Context, string, int) ([]domain.Product, error) {
	return c.products, nil
}

func TestAutocompleteCatalogDown(t *testing.T) {
	w := newWorld()
	w.catalog.err = errors.New("down")
	q, _ := domain.NewSuggestQuery("napa", 0)
	if _, err := w.service(Options{}).Autocomplete(context.Background(), q); !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

// --- Misc ---------------------------------------------------------------------------

func TestNewServiceDefaults(t *testing.T) {
	svc := NewService(Deps{}, Options{})
	if svc.opts.CallTimeout != 800*time.Millisecond || svc.opts.RequestTimeout != 2*time.Second || svc.opts.Logger == nil {
		t.Errorf("defaults = %+v", svc.opts)
	}
	id1, id2 := svc.opts.NewID(), svc.opts.NewID()
	if len(id1) != 36 || id1 == id2 || id1[14] != '4' {
		t.Errorf("search IDs %q, %q are not distinct v4 UUIDs", id1, id2)
	}
}
