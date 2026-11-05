package clients

import (
	"context"
	"reflect"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/adapters/upstreamtest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/ports"
)

func TestClientsMapProtoToSearchTypes(t *testing.T) {
	f := &upstreamtest.Upstream{}
	conn := upstreamtest.Start(t, f)
	ctx := context.Background()
	keys := []domain.Key{{PharmacyID: "ph-1", ProductID: "pr-napa"}, {PharmacyID: "ph-2", ProductID: "pr-napa"}}

	products, err := NewCatalog(conn).ResolveName(ctx, "napa", 5)
	if err != nil {
		t.Fatal(err)
	}
	if want := []domain.Product{{ID: "pr-napa", BrandName: "Napa", GenericName: "Paracetamol", Strength: "500mg"}}; !reflect.DeepEqual(products, want) {
		t.Errorf("catalog = %+v (the controlled product must be dropped)", products)
	}

	nearby, err := NewGeo(conn).FindNearby(ctx, domain.Point{Lat: 27.4, Lng: 89.6}, 3000, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(nearby) != 2 || nearby[0] != (ports.NearbyPharmacy{PharmacyID: "ph-1", DistanceM: 250}) {
		t.Errorf("geo = %+v", nearby)
	}
	if f.GotGeo.GetRadiusM() != 3000 || f.GotGeo.GetLimit() != 50 || f.GotGeo.GetLocation().GetLatitude() != 27.4 {
		t.Errorf("geo request = %v", f.GotGeo)
	}

	stock, err := NewInventory(conn).AvailableStock(ctx, []string{"ph-1", "ph-2"}, []string{"pr-napa"})
	if err != nil {
		t.Fatal(err)
	}
	if !f.GotInventory.GetOnlyAvailable() {
		t.Error("inventory was not asked for available stock only")
	}
	if len(stock) != 2 || stock[0].Available != 4 || !stock[0].ConfirmedAt.Equal(upstreamtest.Confirmed) || !stock[1].ConfirmedAt.IsZero() {
		t.Errorf("inventory = %+v", stock)
	}

	fresh, err := NewFreshness(conn).Scores(ctx, keys)
	if err != nil || fresh[keys[0]] != 0.8 || len(fresh) != 2 {
		t.Errorf("freshness = %v, %v", fresh, err)
	}

	prices, err := NewPricing(conn).Prices(ctx, keys)
	if err != nil || prices[keys[0]] != 4500 || len(prices) != 1 {
		t.Errorf("pricing = %v, %v", prices, err)
	}

	trust, err := NewTrust(conn).Scores(ctx, []string{"ph-1"})
	if err != nil || trust["ph-1"] != 0.7 {
		t.Errorf("trust = %v, %v", trust, err)
	}

	info, err := NewRegistry(conn).Pharmacies(ctx, []string{"ph-1", "ph-2"})
	if err != nil {
		t.Fatal(err)
	}
	if info["ph-1"].Name != "Pharmacy ph-1" || info["ph-1"].Location == nil || info["ph-2"].Location != nil {
		t.Errorf("registry = %+v", info)
	}

	ranked, err := NewRanking(conn).Rank(ctx, []ports.RankCandidate{
		{Key: keys[0], DistanceM: 250, Freshness: 0.8, Trust: 0.7, PriceChhertum: 4500, Available: 4},
		{Key: keys[1], DistanceM: 900, Available: 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) != 2 || ranked[0].Key != keys[1] || ranked[1].Score != 0 {
		t.Errorf("ranking = %+v", ranked)
	}
	c := f.GotRanking.GetCandidates()[0]
	if c.GetDistanceM() != 250 || c.GetFreshnessScore() != 0.8 || c.GetTrustScore() != 0.7 || c.GetPriceChhertum() != 4500 || c.GetQuantityAvailable() != 4 || f.GotRanking.GetWeights() != nil {
		t.Errorf("ranking request candidate = %v (weights must be left to Ranking's defaults)", c)
	}

	subs, err := NewSubstitution(conn).Substitutes(ctx, "pr-napa", 5)
	if err != nil || len(subs) != 1 || subs[0].ID != "pr-alt" {
		t.Errorf("substitution = %+v, %v", subs, err)
	}
}

func TestClientsReturnUpstreamErrors(t *testing.T) {
	names := []string{"catalog", "geo", "inventory", "freshness", "pricing", "trust", "registry", "ranking", "substitution"}
	f := &upstreamtest.Upstream{Down: map[string]bool{}}
	for _, n := range names {
		f.Down[n] = true
	}
	conn := upstreamtest.Start(t, f)
	ctx := context.Background()

	calls := map[string]func() error{
		"catalog":      func() error { _, err := NewCatalog(conn).ResolveName(ctx, "x", 1); return err },
		"geo":          func() error { _, err := NewGeo(conn).FindNearby(ctx, domain.Point{}, 1, 1); return err },
		"inventory":    func() error { _, err := NewInventory(conn).AvailableStock(ctx, nil, nil); return err },
		"freshness":    func() error { _, err := NewFreshness(conn).Scores(ctx, nil); return err },
		"pricing":      func() error { _, err := NewPricing(conn).Prices(ctx, nil); return err },
		"trust":        func() error { _, err := NewTrust(conn).Scores(ctx, nil); return err },
		"registry":     func() error { _, err := NewRegistry(conn).Pharmacies(ctx, nil); return err },
		"ranking":      func() error { _, err := NewRanking(conn).Rank(ctx, nil); return err },
		"substitution": func() error { _, err := NewSubstitution(conn).Substitutes(ctx, "x", 1); return err },
	}
	for _, n := range names {
		if err := calls[n](); status.Code(err) != codes.Unavailable {
			t.Errorf("%s: err = %v, want Unavailable", n, err)
		}
	}
}
