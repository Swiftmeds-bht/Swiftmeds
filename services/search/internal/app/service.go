// Package app is Search's use cases: it orchestrates Catalog, Geo, Inventory,
// Freshness, Pricing, Trust, Pharmacy Registry, Ranking and Substitution.
//
// Required: Catalog, Geo and Inventory. Without them there is no answer, so
// their errors fail the search (ErrUpstream).
// Optional: Freshness, Pricing, Trust, Registry, Ranking and Substitution. If
// one fails or is too slow, the search still answers with neutral values and
// lists the service in Result.Degraded.
package app

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/ports"
)

// ErrUpstream means a required service failed, so no answer can be given.
var ErrUpstream = errors.New("a required service is unavailable")

const (
	// MaxProducts is how many catalogue matches a search shows.
	MaxProducts = 5
	// MaxPharmacies is how many nearby pharmacies a search considers.
	MaxPharmacies = 100
	// MaxSubstitutes is how many alternative products a search shows.
	MaxSubstitutes = 5
	// NeutralTrust is used when Trust does not answer: neither reward nor penalty.
	NeutralTrust = 0.5
)

// Names used in Result.Degraded.
const (
	DepFreshness    = "freshness"
	DepPricing      = "pricing"
	DepTrust        = "trust"
	DepRegistry     = "pharmacy-registry"
	DepRanking      = "ranking"
	DepSubstitution = "substitution"
	DepSearchEvents = "search-events"
)

type Deps struct {
	Catalog      ports.Catalog
	Geo          ports.Geo
	Inventory    ports.Inventory
	Freshness    ports.Freshness
	Pricing      ports.Pricing
	Trust        ports.Trust
	Registry     ports.Registry
	Ranking      ports.Ranking
	Substitution ports.Substitution
	Events       ports.SearchEvents
}

type Options struct {
	// CallTimeout limits each downstream call. Default 800ms.
	CallTimeout time.Duration
	// RequestTimeout limits a whole search. Default 2s (the proposal's target).
	RequestTimeout time.Duration
	Logger         *slog.Logger
	// NewID makes search IDs; tests make it predictable.
	NewID func() string
}

type Service struct {
	deps Deps
	opts Options
}

func NewService(deps Deps, opts Options) *Service {
	if opts.CallTimeout <= 0 {
		opts.CallTimeout = 800 * time.Millisecond
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 2 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if opts.NewID == nil {
		opts.NewID = newUUID
	}
	return &Service{deps: deps, opts: opts}
}

// degraded collects the optional services that did not answer. Safe for concurrent use.
type degraded struct {
	mu   sync.Mutex
	deps []string
}

func (d *degraded) add(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !slices.Contains(d.deps, name) {
		d.deps = append(d.deps, name)
	}
}

func (d *degraded) list() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := slices.Clone(d.deps)
	slices.Sort(out)
	return out
}

// call runs one downstream call with its own timeout.
func (s *Service) call(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, s.opts.CallTimeout)
	defer cancel()
	return fn(ctx)
}

// Search answers "which nearby pharmacies have this medicine?".
func (s *Service) Search(ctx context.Context, q domain.Query) (domain.Result, error) {
	res, err := s.search(ctx, q)
	if err != nil {
		// Callers only see "temporarily unavailable"; keep the real cause in the log.
		s.opts.Logger.ErrorContext(ctx, "search failed", slog.String("error", err.Error()))
	}
	return res, err
}

func (s *Service) search(ctx context.Context, q domain.Query) (domain.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, s.opts.RequestTimeout)
	defer cancel()
	res := domain.Result{SearchID: s.opts.NewID()}
	deg := &degraded{}

	// 1. Catalog and Geo are independent: ask both at once.
	var (
		products           []domain.Product
		nearby             []ports.NearbyPharmacy
		catalogErr, geoErr error
		wg                 sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		catalogErr = s.call(ctx, func(ctx context.Context) (err error) {
			products, err = s.deps.Catalog.ResolveName(ctx, q.Text, MaxProducts)
			return err
		})
	}()
	go func() {
		defer wg.Done()
		geoErr = s.call(ctx, func(ctx context.Context) (err error) {
			nearby, err = s.deps.Geo.FindNearby(ctx, q.Center, q.RadiusM, MaxPharmacies)
			return err
		})
	}()
	wg.Wait()
	if catalogErr != nil {
		return domain.Result{}, fmt.Errorf("%w: catalog: %w", ErrUpstream, catalogErr)
	}
	if geoErr != nil {
		return domain.Result{}, fmt.Errorf("%w: geo: %w", ErrUpstream, geoErr)
	}
	if len(products) > MaxProducts {
		products = products[:MaxProducts]
	}

	// 2. Offers for what the patient asked for.
	results, err := s.offers(ctx, products, nearby, q.MaxOffers, deg)
	if err != nil {
		return domain.Result{}, err
	}
	res.Results = results

	// 3. Nothing in stock nearby: suggest same-molecule alternatives that are.
	if len(products) > 0 && len(nearby) > 0 && !domain.HasOffers(results) && s.deps.Substitution != nil {
		subs := s.substitutes(ctx, products, deg)
		if len(subs) > 0 {
			subResults, err := s.offers(ctx, subs, nearby, q.MaxOffers, deg)
			if err != nil {
				return domain.Result{}, err
			}
			for _, r := range subResults {
				if len(r.Offers) > 0 {
					res.Substitutes = append(res.Substitutes, r)
				}
			}
		}
	}

	res.Degraded = deg.list()
	if s.deps.Events != nil {
		for _, result := range res.Results {
			if err := s.deps.Events.SearchRecorded(ctx, res.SearchID, q.Dzongkhag, result); err != nil {
				s.opts.Logger.ErrorContext(ctx, "search event publish failed",
					slog.String("search_id", res.SearchID),
					slog.String("product_id", result.ID),
					slog.String("error", err.Error()))
				deg.add(DepSearchEvents)
			}
		}
		res.Degraded = deg.list()
	}
	if len(res.Degraded) > 0 {
		s.opts.Logger.WarnContext(ctx, "search answered with partial data",
			slog.String("search_id", res.SearchID), slog.Any("degraded", res.Degraded))
	}
	return res, nil
}

// substitutes asks Substitution about every matched product at once and
// returns the distinct alternatives (never one of the matched products).
func (s *Service) substitutes(ctx context.Context, products []domain.Product, deg *degraded) []domain.Product {
	lists := make([][]domain.Product, len(products))
	var wg sync.WaitGroup
	for i, p := range products {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.call(ctx, func(ctx context.Context) (err error) {
				lists[i], err = s.deps.Substitution.Substitutes(ctx, p.ID, MaxSubstitutes)
				return err
			})
			if err != nil {
				deg.add(DepSubstitution)
			}
		}()
	}
	wg.Wait()

	seen := make(map[string]bool, len(products))
	for _, p := range products {
		seen[p.ID] = true
	}
	var out []domain.Product
	for _, list := range lists {
		for _, p := range list {
			if !seen[p.ID] && len(out) < MaxSubstitutes {
				seen[p.ID] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// offers finds, enriches and ranks the in-stock offers for products.
func (s *Service) offers(ctx context.Context, products []domain.Product, nearby []ports.NearbyPharmacy, maxOffers int, deg *degraded) ([]domain.ProductResult, error) {
	results := make([]domain.ProductResult, len(products))
	for i, p := range products {
		results[i] = domain.ProductResult{Product: p, Offers: []domain.Offer{}}
	}
	if len(products) == 0 || len(nearby) == 0 {
		return results, nil
	}

	distance := make(map[string]int32, len(nearby))
	pharmacyIDs := make([]string, 0, len(nearby))
	for _, n := range nearby {
		distance[n.PharmacyID] = n.DistanceM
		pharmacyIDs = append(pharmacyIDs, n.PharmacyID)
	}
	productIDs := make([]string, 0, len(products))
	for _, p := range products {
		productIDs = append(productIDs, p.ID)
	}

	// Required: who has it in stock?
	var stock []ports.Stock
	if err := s.call(ctx, func(ctx context.Context) (err error) {
		stock, err = s.deps.Inventory.AvailableStock(ctx, pharmacyIDs, productIDs)
		return err
	}); err != nil {
		return nil, fmt.Errorf("%w: inventory: %w", ErrUpstream, err)
	}
	var inStock []ports.Stock
	for _, st := range stock {
		if _, near := distance[st.PharmacyID]; near && st.Available > 0 {
			inStock = append(inStock, st)
		}
	}
	if len(inStock) == 0 {
		return results, nil
	}

	keys := make([]domain.Key, 0, len(inStock))
	phSet := map[string]bool{}
	var offerPharmacies []string
	for _, st := range inStock {
		keys = append(keys, st.Key)
		if !phSet[st.PharmacyID] {
			phSet[st.PharmacyID] = true
			offerPharmacies = append(offerPharmacies, st.PharmacyID)
		}
	}

	// Optional enrichment, all at once.
	var (
		fresh  map[domain.Key]float64
		prices map[domain.Key]int64
		trust  map[string]float64
		info   map[string]ports.PharmacyInfo
		wg     sync.WaitGroup
	)
	optional := func(name string, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.call(ctx, fn); err != nil {
				deg.add(name)
			}
		}()
	}
	optional(DepFreshness, func(ctx context.Context) (err error) {
		fresh, err = s.deps.Freshness.Scores(ctx, keys)
		return err
	})
	optional(DepPricing, func(ctx context.Context) (err error) {
		prices, err = s.deps.Pricing.Prices(ctx, keys)
		return err
	})
	optional(DepTrust, func(ctx context.Context) (err error) {
		trust, err = s.deps.Trust.Scores(ctx, offerPharmacies)
		return err
	})
	optional(DepRegistry, func(ctx context.Context) (err error) {
		info, err = s.deps.Registry.Pharmacies(ctx, offerPharmacies)
		return err
	})
	wg.Wait()

	offers := make(map[domain.Key]domain.Offer, len(inStock))
	candidates := make([]ports.RankCandidate, 0, len(inStock))
	for _, st := range inStock {
		t, ok := trust[st.PharmacyID]
		if !ok {
			t = NeutralTrust
		}
		o := domain.Offer{
			PharmacyID:    st.PharmacyID,
			DistanceM:     distance[st.PharmacyID],
			Quantity:      st.Available,
			PriceChhertum: prices[st.Key], // 0 when unknown
			Freshness:     fresh[st.Key],  // 0 (never confirmed) when unknown
			Trust:         t,
			ConfirmedAt:   st.ConfirmedAt,
		}
		if pi, ok := info[st.PharmacyID]; ok {
			o.PharmacyName, o.Address, o.Location = pi.Name, pi.Address, pi.Location
		}
		offers[st.Key] = o
		candidates = append(candidates, ports.RankCandidate{
			Key: st.Key, DistanceM: o.DistanceM, Freshness: o.Freshness, Trust: o.Trust,
			PriceChhertum: o.PriceChhertum, Available: o.Quantity,
		})
	}

	// Optional: rank. If Ranking fails, fall back to nearest first.
	var ranked []ports.Ranked
	if err := s.call(ctx, func(ctx context.Context) (err error) {
		ranked, err = s.deps.Ranking.Rank(ctx, candidates)
		return err
	}); err != nil {
		deg.add(DepRanking)
		ranked = nearestFirst(candidates)
	}

	byProduct := make(map[string][]domain.Offer, len(products))
	for _, r := range ranked {
		o, ok := offers[r.Key]
		if !ok || len(byProduct[r.ProductID]) >= maxOffers {
			continue
		}
		o.RankScore = r.Score
		byProduct[r.ProductID] = append(byProduct[r.ProductID], o)
	}
	for i := range results {
		if os := byProduct[results[i].ID]; os != nil {
			results[i].Offers = os
		}
	}
	return results, nil
}

// nearestFirst is the fallback order when Ranking is unavailable.
func nearestFirst(cs []ports.RankCandidate) []ports.Ranked {
	sorted := slices.Clone(cs)
	slices.SortFunc(sorted, func(a, b ports.RankCandidate) int {
		return cmp.Or(cmp.Compare(a.DistanceM, b.DistanceM), cmp.Compare(a.PharmacyID, b.PharmacyID), cmp.Compare(a.ProductID, b.ProductID))
	})
	out := make([]ports.Ranked, len(sorted))
	for i, c := range sorted {
		out[i] = ports.Ranked{Key: c.Key}
	}
	return out
}

// Autocomplete suggests product names while the patient types.
func (s *Service) Autocomplete(ctx context.Context, q domain.SuggestQuery) ([]domain.Product, error) {
	var products []domain.Product
	if err := s.call(ctx, func(ctx context.Context) (err error) {
		products, err = s.deps.Catalog.ResolveName(ctx, q.Prefix, q.Limit)
		return err
	}); err != nil {
		err = fmt.Errorf("%w: catalog: %w", ErrUpstream, err)
		s.opts.Logger.ErrorContext(ctx, "autocomplete failed", slog.String("error", err.Error()))
		return nil, err
	}
	if len(products) > q.Limit {
		products = products[:q.Limit]
	}
	return products, nil
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
