// Package upstreamtest is a fake of all nine services Search calls, served
// over an in-memory gRPC connection. Used by the clients and handler tests.
package upstreamtest

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

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
)

// Upstream plays all nine downstream services on one in-memory server.
// Set Down["catalog"] (etc.) to make that service return Unavailable.
type Upstream struct {
	catalogv1.UnimplementedCatalogServiceServer
	geov1.UnimplementedGeoServiceServer
	inventoryv1.UnimplementedInventoryServiceServer
	freshnessv1.UnimplementedFreshnessServiceServer
	pricingv1.UnimplementedPricingServiceServer
	trustv1.UnimplementedTrustServiceServer
	pharmacyregistryv1.UnimplementedPharmacyRegistryServiceServer
	rankingv1.UnimplementedRankingServiceServer
	substitutionv1.UnimplementedSubstitutionServiceServer

	Down map[string]bool

	GotInventory *inventoryv1.BatchGetStockRequest
	GotRanking   *rankingv1.RankOffersRequest
	GotGeo       *geov1.FindNearbyPharmaciesRequest
}

var Confirmed = time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)

func (f *Upstream) check(name string) error {
	if f.Down[name] {
		return status.Error(codes.Unavailable, name+" is down")
	}
	return nil
}

func (f *Upstream) ResolveName(_ context.Context, req *catalogv1.ResolveNameRequest) (*catalogv1.ResolveNameResponse, error) {
	if err := f.check("catalog"); err != nil {
		return nil, err
	}
	all := []*catalogv1.Product{
		{ProductId: "pr-napa", BrandName: "Napa", GenericName: "Paracetamol", Strength: "500mg"},
		{ProductId: "pr-secret", BrandName: "Napa Strong", GenericName: "Codeine", Strength: "30mg", Controlled: true},
	}
	if int(req.GetLimit()) < len(all) {
		all = all[:req.GetLimit()]
	}
	return &catalogv1.ResolveNameResponse{Products: all}, nil
}

func (f *Upstream) FindNearbyPharmacies(_ context.Context, req *geov1.FindNearbyPharmaciesRequest) (*geov1.FindNearbyPharmaciesResponse, error) {
	if err := f.check("geo"); err != nil {
		return nil, err
	}
	f.GotGeo = req
	return &geov1.FindNearbyPharmaciesResponse{Pharmacies: []*geov1.NearbyPharmacy{
		{PharmacyId: "ph-1", DistanceM: 250}, {PharmacyId: "ph-2", DistanceM: 900},
	}}, nil
}

func (f *Upstream) BatchGetStock(_ context.Context, req *inventoryv1.BatchGetStockRequest) (*inventoryv1.BatchGetStockResponse, error) {
	if err := f.check("inventory"); err != nil {
		return nil, err
	}
	f.GotInventory = req
	return &inventoryv1.BatchGetStockResponse{Stock: []*inventoryv1.StockLevel{
		{PharmacyId: "ph-1", ProductId: "pr-napa", QuantityAvailable: 4, ConfirmedAt: timestamppb.New(Confirmed)},
		{PharmacyId: "ph-2", ProductId: "pr-napa", QuantityAvailable: 7},
	}}, nil
}

func (f *Upstream) GetFreshness(_ context.Context, req *freshnessv1.GetFreshnessRequest) (*freshnessv1.GetFreshnessResponse, error) {
	if err := f.check("freshness"); err != nil {
		return nil, err
	}
	out := &freshnessv1.GetFreshnessResponse{}
	for _, k := range req.GetKeys() {
		out.Scores = append(out.Scores, &freshnessv1.FreshnessScore{Key: k, Score: 0.8})
	}
	return out, nil
}

func (f *Upstream) BatchGetPrices(_ context.Context, req *pricingv1.BatchGetPricesRequest) (*pricingv1.BatchGetPricesResponse, error) {
	if err := f.check("pricing"); err != nil {
		return nil, err
	}
	out := &pricingv1.BatchGetPricesResponse{}
	for _, k := range req.GetKeys() {
		if k.GetPharmacyId() == "ph-1" {
			out.Prices = append(out.Prices, &pricingv1.Price{Key: k, PriceChhertum: 4500})
		}
	}
	return out, nil
}

func (f *Upstream) GetTrustScores(_ context.Context, req *trustv1.GetTrustScoresRequest) (*trustv1.GetTrustScoresResponse, error) {
	if err := f.check("trust"); err != nil {
		return nil, err
	}
	out := &trustv1.GetTrustScoresResponse{}
	for _, id := range req.GetPharmacyIds() {
		out.Scores = append(out.Scores, &trustv1.TrustScore{PharmacyId: id, Score: 0.7})
	}
	return out, nil
}

func (f *Upstream) BatchGetPharmacies(_ context.Context, req *pharmacyregistryv1.BatchGetPharmaciesRequest) (*pharmacyregistryv1.BatchGetPharmaciesResponse, error) {
	if err := f.check("registry"); err != nil {
		return nil, err
	}
	out := &pharmacyregistryv1.BatchGetPharmaciesResponse{}
	for _, id := range req.GetPharmacyIds() {
		p := &pharmacyregistryv1.Pharmacy{PharmacyId: id, Name: "Pharmacy " + id, Address: "Address " + id}
		if id == "ph-1" {
			p.Location = &commonv1.GeoPoint{Latitude: 27.47, Longitude: 89.63}
		}
		out.Pharmacies = append(out.Pharmacies, p)
	}
	return out, nil
}

// RankOffers puts the farther pharmacy first, so tests can tell Ranking's order
// apart from the nearest-first fallback.
func (f *Upstream) RankOffers(_ context.Context, req *rankingv1.RankOffersRequest) (*rankingv1.RankOffersResponse, error) {
	if err := f.check("ranking"); err != nil {
		return nil, err
	}
	f.GotRanking = req
	out := &rankingv1.RankOffersResponse{}
	for i := len(req.GetCandidates()) - 1; i >= 0; i-- {
		c := req.GetCandidates()[i]
		out.Offers = append(out.Offers, &rankingv1.RankedOffer{PharmacyId: c.GetPharmacyId(), ProductId: c.GetProductId(), Score: float64(i) / 10})
	}
	return out, nil
}

func (f *Upstream) GetSubstitutes(_ context.Context, req *substitutionv1.GetSubstitutesRequest) (*substitutionv1.GetSubstitutesResponse, error) {
	if err := f.check("substitution"); err != nil {
		return nil, err
	}
	return &substitutionv1.GetSubstitutesResponse{Substitutes: []*substitutionv1.Substitute{
		{ProductId: "pr-alt", BrandName: "Alt", GenericName: "Paracetamol", Strength: "500mg"},
	}}, nil
}

// Start serves u over bufconn and returns a client connection.
func Start(t *testing.T, f *Upstream) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	catalogv1.RegisterCatalogServiceServer(srv, f)
	geov1.RegisterGeoServiceServer(srv, f)
	inventoryv1.RegisterInventoryServiceServer(srv, f)
	freshnessv1.RegisterFreshnessServiceServer(srv, f)
	pricingv1.RegisterPricingServiceServer(srv, f)
	trustv1.RegisterTrustServiceServer(srv, f)
	pharmacyregistryv1.RegisterPharmacyRegistryServiceServer(srv, f)
	rankingv1.RegisterRankingServiceServer(srv, f)
	substitutionv1.RegisterSubstitutionServiceServer(srv, f)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///upstream",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
