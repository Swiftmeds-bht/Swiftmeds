package grpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	searchv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/search/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/adapters/clients"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/adapters/upstreamtest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/app"
)

var thimphu = &commonv1.GeoPoint{Latitude: 27.4728, Longitude: 89.639}

// startSearch wires the real handler, app and clients against the fake
// upstream, serves it over bufconn, and returns a SearchService client.
func startSearch(t *testing.T, up *upstreamtest.Upstream) searchv1.SearchServiceClient {
	t.Helper()
	upConn := upstreamtest.Start(t, up)
	svc := app.NewService(app.Deps{
		Catalog: clients.NewCatalog(upConn), Geo: clients.NewGeo(upConn), Inventory: clients.NewInventory(upConn),
		Freshness: clients.NewFreshness(upConn), Pricing: clients.NewPricing(upConn), Trust: clients.NewTrust(upConn),
		Registry: clients.NewRegistry(upConn), Ranking: clients.NewRanking(upConn), Substitution: clients.NewSubstitution(upConn),
	}, app.Options{NewID: func() string { return "s-1" }})

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	searchv1.RegisterSearchServiceServer(srv, NewHandler(svc))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///search",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return searchv1.NewSearchServiceClient(conn)
}

func TestSearchMedicineEndToEndOverGRPC(t *testing.T) {
	c := startSearch(t, &upstreamtest.Upstream{})
	var header metadata.MD
	resp, err := c.SearchMedicine(context.Background(), &searchv1.SearchMedicineRequest{
		Query: "napa", Location: thimphu, Dzongkhag: "  Thimphu  ",
	}, grpc.Header(&header))
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetSearchId() != "s-1" || len(resp.GetResults()) != 1 || len(resp.GetSubstitutes()) != 0 {
		t.Fatalf("response = %v", resp)
	}
	r := resp.GetResults()[0]
	if r.GetProductId() != "pr-napa" || r.GetBrandName() != "Napa" || r.GetGenericName() != "Paracetamol" || r.GetStrength() != "500mg" {
		t.Errorf("product = %v", r)
	}
	offers := r.GetOffers()
	if len(offers) != 2 {
		t.Fatalf("offers = %v", offers)
	}
	// The fake Ranking puts the farther pharmacy (ph-2) first.
	if offers[0].GetPharmacyId() != "ph-2" || offers[1].GetPharmacyId() != "ph-1" {
		t.Errorf("order = %s, %s; want Ranking's order ph-2, ph-1", offers[0].GetPharmacyId(), offers[1].GetPharmacyId())
	}
	ph1 := offers[1]
	if ph1.GetPharmacyName() != "Pharmacy ph-1" || ph1.GetAddress() != "Address ph-1" || ph1.GetDistanceM() != 250 ||
		ph1.GetQuantityAvailable() != 4 || ph1.GetPriceChhertum() != 4500 || ph1.GetFreshnessScore() != 0.8 ||
		ph1.GetTrustScore() != 0.7 || ph1.GetLocation().GetLatitude() != 27.47 ||
		!ph1.GetStockConfirmedAt().AsTime().Equal(upstreamtest.Confirmed) {
		t.Errorf("ph-1 offer = %v", ph1)
	}
	ph2 := offers[0]
	if ph2.GetPriceChhertum() != 0 || ph2.GetLocation() != nil || ph2.GetStockConfirmedAt() != nil {
		t.Errorf("ph-2 offer = %v; want no price, no location, no confirmation time", ph2)
	}
	if got := header.Get(DegradedHeader); len(got) != 0 {
		t.Errorf("degraded header = %v, want none when everything answered", got)
	}
}

func TestSearchMedicinePartialResultsSetTheDegradedHeader(t *testing.T) {
	c := startSearch(t, &upstreamtest.Upstream{Down: map[string]bool{"freshness": true, "ranking": true}})
	var header metadata.MD
	resp, err := c.SearchMedicine(context.Background(), &searchv1.SearchMedicineRequest{Query: "napa", Location: thimphu}, grpc.Header(&header))
	if err != nil {
		t.Fatalf("an optional service being down must not fail the search: %v", err)
	}
	if got := header.Get(DegradedHeader); len(got) != 1 || got[0] != "freshness,ranking" {
		t.Errorf("degraded header = %v, want [freshness,ranking]", got)
	}
	offers := resp.GetResults()[0].GetOffers()
	if offers[0].GetPharmacyId() != "ph-1" || offers[0].GetFreshnessScore() != 0 {
		t.Errorf("offers = %v; want nearest-first fallback and no freshness", offers)
	}
}

func TestSearchMedicineRequiredServiceDownIsUnavailable(t *testing.T) {
	for _, dep := range []string{"catalog", "geo", "inventory"} {
		c := startSearch(t, &upstreamtest.Upstream{Down: map[string]bool{dep: true}})
		_, err := c.SearchMedicine(context.Background(), &searchv1.SearchMedicineRequest{Query: "napa", Location: thimphu})
		if status.Code(err) != codes.Unavailable {
			t.Errorf("%s down: code = %v, want Unavailable", dep, status.Code(err))
		}
	}
}

func TestSearchMedicineInvalidRequests(t *testing.T) {
	c := startSearch(t, &upstreamtest.Upstream{})
	cases := map[string]*searchv1.SearchMedicineRequest{
		"no location":      {Query: "napa"},
		"query too short":  {Query: "n", Location: thimphu},
		"radius too small": {Query: "napa", Location: thimphu, RadiusM: 100},
		"too many offers":  {Query: "napa", Location: thimphu, MaxOffers: 51},
		"bad latitude":     {Query: "napa", Location: &commonv1.GeoPoint{Latitude: 91}},
	}
	for name, req := range cases {
		if _, err := c.SearchMedicine(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: code = %v, want InvalidArgument", name, status.Code(err))
		}
	}
}

func TestAutocompleteOverGRPC(t *testing.T) {
	c := startSearch(t, &upstreamtest.Upstream{})
	resp, err := c.Autocomplete(context.Background(), &searchv1.AutocompleteRequest{Prefix: "na", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	s := resp.GetSuggestions()
	if len(s) != 1 || s[0].GetProductId() != "pr-napa" || s[0].GetDisplayName() != "Napa 500mg (Paracetamol)" {
		t.Errorf("suggestions = %v (controlled products must never be suggested)", s)
	}

	if _, err := c.Autocomplete(context.Background(), &searchv1.AutocompleteRequest{Prefix: "n"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("short prefix: code = %v", status.Code(err))
	}
	down := startSearch(t, &upstreamtest.Upstream{Down: map[string]bool{"catalog": true}})
	if _, err := down.Autocomplete(context.Background(), &searchv1.AutocompleteRequest{Prefix: "na"}); status.Code(err) != codes.Unavailable {
		t.Errorf("catalog down: code = %v", status.Code(err))
	}
}

func TestToStatus(t *testing.T) {
	cases := map[error]codes.Code{
		context.DeadlineExceeded: codes.DeadlineExceeded,
		app.ErrUpstream:          codes.Unavailable,
		errors.New("boom"):       codes.Internal,
	}
	for err, want := range cases {
		if got := status.Code(toStatus(err)); got != want {
			t.Errorf("toStatus(%v) = %v, want %v", err, got, want)
		}
	}
	if msg := status.Convert(toStatus(errors.New("pq: secret details"))).Message(); msg != "internal error" {
		t.Errorf("internal error leaks details: %q", msg)
	}
}

// Sanity check that the end-to-end search stays far below the 2 s target.
func TestSearchMedicineIsFast(t *testing.T) {
	c := startSearch(t, &upstreamtest.Upstream{})
	start := time.Now()
	if _, err := c.SearchMedicine(context.Background(), &searchv1.SearchMedicineRequest{Query: "napa", Location: thimphu}); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("search took %v, want < 2s", took)
	}
}
