// Package clients holds Search's outbound adapters: one small gRPC client per
// downstream service, each implementing a port. They only translate between
// proto messages and Search's types.
package clients

import (
	"context"

	"google.golang.org/grpc"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	freshnessv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/freshness/v1"
	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	rankingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/ranking/v1"
	substitutionv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/substitution/v1"
	trustv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/trust/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/ports"
)

var (
	_ ports.Catalog      = Catalog{}
	_ ports.Geo          = Geo{}
	_ ports.Inventory    = Inventory{}
	_ ports.Freshness    = Freshness{}
	_ ports.Pricing      = Pricing{}
	_ ports.Trust        = Trust{}
	_ ports.Registry     = Registry{}
	_ ports.Ranking      = Ranking{}
	_ ports.Substitution = Substitution{}
)

func toKeys(keys []domain.Key) []*commonv1.StockKey {
	out := make([]*commonv1.StockKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, &commonv1.StockKey{PharmacyId: k.PharmacyID, ProductId: k.ProductID})
	}
	return out
}

func fromKey(k *commonv1.StockKey) domain.Key {
	return domain.Key{PharmacyID: k.GetPharmacyId(), ProductID: k.GetProductId()}
}

func toProduct(id, brand, generic, strength string) domain.Product {
	return domain.Product{ID: id, BrandName: brand, GenericName: generic, Strength: strength}
}

// Catalog ---------------------------------------------------------------------

type Catalog struct {
	C catalogv1.CatalogServiceClient
}

func NewCatalog(cc grpc.ClientConnInterface) Catalog {
	return Catalog{catalogv1.NewCatalogServiceClient(cc)}
}

func (c Catalog) ResolveName(ctx context.Context, query string, limit int) ([]domain.Product, error) {
	resp, err := c.C.ResolveName(ctx, &catalogv1.ResolveNameRequest{Query: query, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Product, 0, len(resp.GetProducts()))
	for _, p := range resp.GetProducts() {
		if p.GetControlled() { // Catalog already excludes them; never show one if it slips through.
			continue
		}
		out = append(out, toProduct(p.GetProductId(), p.GetBrandName(), p.GetGenericName(), p.GetStrength()))
	}
	return out, nil
}

// Geo -------------------------------------------------------------------------

type Geo struct{ C geov1.GeoServiceClient }

func NewGeo(cc grpc.ClientConnInterface) Geo { return Geo{geov1.NewGeoServiceClient(cc)} }

func (g Geo) FindNearby(ctx context.Context, center domain.Point, radiusM int32, limit int) ([]ports.NearbyPharmacy, error) {
	resp, err := g.C.FindNearbyPharmacies(ctx, &geov1.FindNearbyPharmaciesRequest{
		Location: &commonv1.GeoPoint{Latitude: center.Lat, Longitude: center.Lng},
		RadiusM:  radiusM,
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ports.NearbyPharmacy, 0, len(resp.GetPharmacies()))
	for _, p := range resp.GetPharmacies() {
		out = append(out, ports.NearbyPharmacy{PharmacyID: p.GetPharmacyId(), DistanceM: p.GetDistanceM()})
	}
	return out, nil
}

// Inventory -------------------------------------------------------------------

type Inventory struct {
	C inventoryv1.InventoryServiceClient
}

func NewInventory(cc grpc.ClientConnInterface) Inventory {
	return Inventory{inventoryv1.NewInventoryServiceClient(cc)}
}

func (i Inventory) AvailableStock(ctx context.Context, pharmacyIDs, productIDs []string) ([]ports.Stock, error) {
	resp, err := i.C.BatchGetStock(ctx, &inventoryv1.BatchGetStockRequest{
		PharmacyIds: pharmacyIDs, ProductIds: productIDs, OnlyAvailable: true,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ports.Stock, 0, len(resp.GetStock()))
	for _, s := range resp.GetStock() {
		st := ports.Stock{
			Key:       domain.Key{PharmacyID: s.GetPharmacyId(), ProductID: s.GetProductId()},
			Available: s.GetQuantityAvailable(),
		}
		if s.GetConfirmedAt() != nil {
			st.ConfirmedAt = s.GetConfirmedAt().AsTime()
		}
		out = append(out, st)
	}
	return out, nil
}

// Freshness -------------------------------------------------------------------

type Freshness struct {
	C freshnessv1.FreshnessServiceClient
}

func NewFreshness(cc grpc.ClientConnInterface) Freshness {
	return Freshness{freshnessv1.NewFreshnessServiceClient(cc)}
}

func (f Freshness) Scores(ctx context.Context, keys []domain.Key) (map[domain.Key]float64, error) {
	resp, err := f.C.GetFreshness(ctx, &freshnessv1.GetFreshnessRequest{Keys: toKeys(keys)})
	if err != nil {
		return nil, err
	}
	out := make(map[domain.Key]float64, len(resp.GetScores()))
	for _, s := range resp.GetScores() {
		out[fromKey(s.GetKey())] = s.GetScore()
	}
	return out, nil
}

// Pricing ---------------------------------------------------------------------

type Pricing struct {
	C pricingv1.PricingServiceClient
}

func NewPricing(cc grpc.ClientConnInterface) Pricing {
	return Pricing{pricingv1.NewPricingServiceClient(cc)}
}

func (p Pricing) Prices(ctx context.Context, keys []domain.Key) (map[domain.Key]int64, error) {
	resp, err := p.C.BatchGetPrices(ctx, &pricingv1.BatchGetPricesRequest{Keys: toKeys(keys)})
	if err != nil {
		return nil, err
	}
	out := make(map[domain.Key]int64, len(resp.GetPrices()))
	for _, pr := range resp.GetPrices() {
		out[fromKey(pr.GetKey())] = pr.GetPriceChhertum()
	}
	return out, nil
}

// Trust -----------------------------------------------------------------------

type Trust struct{ C trustv1.TrustServiceClient }

func NewTrust(cc grpc.ClientConnInterface) Trust { return Trust{trustv1.NewTrustServiceClient(cc)} }

func (t Trust) Scores(ctx context.Context, pharmacyIDs []string) (map[string]float64, error) {
	resp, err := t.C.GetTrustScores(ctx, &trustv1.GetTrustScoresRequest{PharmacyIds: pharmacyIDs})
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(resp.GetScores()))
	for _, s := range resp.GetScores() {
		out[s.GetPharmacyId()] = s.GetScore()
	}
	return out, nil
}

// Pharmacy Registry -----------------------------------------------------------

type Registry struct {
	C pharmacyregistryv1.PharmacyRegistryServiceClient
}

func NewRegistry(cc grpc.ClientConnInterface) Registry {
	return Registry{pharmacyregistryv1.NewPharmacyRegistryServiceClient(cc)}
}

func (r Registry) Pharmacies(ctx context.Context, pharmacyIDs []string) (map[string]ports.PharmacyInfo, error) {
	resp, err := r.C.BatchGetPharmacies(ctx, &pharmacyregistryv1.BatchGetPharmaciesRequest{PharmacyIds: pharmacyIDs})
	if err != nil {
		return nil, err
	}
	out := make(map[string]ports.PharmacyInfo, len(resp.GetPharmacies()))
	for _, p := range resp.GetPharmacies() {
		info := ports.PharmacyInfo{Name: p.GetName(), Address: p.GetAddress()}
		if l := p.GetLocation(); l != nil {
			info.Location = &domain.Point{Lat: l.GetLatitude(), Lng: l.GetLongitude()}
		}
		out[p.GetPharmacyId()] = info
	}
	return out, nil
}

// Ranking ---------------------------------------------------------------------

type Ranking struct {
	C rankingv1.RankingServiceClient
}

func NewRanking(cc grpc.ClientConnInterface) Ranking {
	return Ranking{rankingv1.NewRankingServiceClient(cc)}
}

// Rank sends no weights, so Ranking uses its configured defaults.
func (r Ranking) Rank(ctx context.Context, cs []ports.RankCandidate) ([]ports.Ranked, error) {
	req := &rankingv1.RankOffersRequest{Candidates: make([]*rankingv1.Candidate, 0, len(cs))}
	for _, c := range cs {
		req.Candidates = append(req.Candidates, &rankingv1.Candidate{
			PharmacyId: c.PharmacyID, ProductId: c.ProductID, DistanceM: c.DistanceM,
			FreshnessScore: c.Freshness, TrustScore: c.Trust, PriceChhertum: c.PriceChhertum,
			QuantityAvailable: c.Available,
		})
	}
	resp, err := r.C.RankOffers(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make([]ports.Ranked, 0, len(resp.GetOffers()))
	for _, o := range resp.GetOffers() {
		out = append(out, ports.Ranked{
			Key:   domain.Key{PharmacyID: o.GetPharmacyId(), ProductID: o.GetProductId()},
			Score: o.GetScore(),
		})
	}
	return out, nil
}

// Substitution ----------------------------------------------------------------

type Substitution struct {
	C substitutionv1.SubstitutionServiceClient
}

func NewSubstitution(cc grpc.ClientConnInterface) Substitution {
	return Substitution{substitutionv1.NewSubstitutionServiceClient(cc)}
}

func (s Substitution) Substitutes(ctx context.Context, productID string, limit int) ([]domain.Product, error) {
	resp, err := s.C.GetSubstitutes(ctx, &substitutionv1.GetSubstitutesRequest{ProductId: productID, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Product, 0, len(resp.GetSubstitutes()))
	for _, p := range resp.GetSubstitutes() {
		out = append(out, toProduct(p.GetProductId(), p.GetBrandName(), p.GetGenericName(), p.GetStrength()))
	}
	return out, nil
}
