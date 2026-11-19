package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

type lookupCatalog struct{ asked string }

func (c *lookupCatalog) GetProduct(_ context.Context, req *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error) {
	c.asked = req.GetProductId()
	if req.GetProductId() != "p1" {
		return nil, status.Error(codes.NotFound, "no such product")
	}
	return &catalogv1.GetProductResponse{Product: &catalogv1.Product{
		ProductId: "p1", BrandName: "Demo Paracetamol 500", GenericName: "Paracetamol", Strength: "500mg", Controlled: true,
	}}, nil
}

type lookupRegistry struct{ asked string }

func (c *lookupRegistry) GetPharmacy(_ context.Context, req *pharmacyregistryv1.GetPharmacyRequest) (*pharmacyregistryv1.GetPharmacyResponse, error) {
	c.asked = req.GetPharmacyId()
	if req.GetPharmacyId() != "ph1" {
		return nil, status.Error(codes.NotFound, "no such pharmacy")
	}
	return &pharmacyregistryv1.GetPharmacyResponse{Pharmacy: &pharmacyregistryv1.Pharmacy{
		PharmacyId: "ph1", OwnerUserId: "owner-secret-id", LicenseNumber: "DEMO-TIM-001", Name: "Demo Pharmacy Thimphu 01", Address: "Norzin Lam",
	}}, nil
}

// denyAll fails any token check, so a 200 proves the route needs no token.
type denyAll struct{}

func (denyAll) ValidateToken(context.Context, string) (ports.Identity, error) {
	return ports.Identity{}, status.Error(codes.Unauthenticated, "no")
}

func lookupRouter(catalog *lookupCatalog, registry *lookupRegistry) http.Handler {
	return NewRouter(Config{CORSOrigins: defaultCORSOrigins}, slog.Default(),
		WithAuth(denyAll{}), WithProducts(catalog), WithPharmacyRegistry(registry))
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestProductLookupIsPublicAndFollowsTheContract(t *testing.T) {
	catalog := &lookupCatalog{}
	rec := get(t, lookupRouter(catalog, &lookupRegistry{}), "/api/v1/products/p1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 without a token: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if catalog.asked != "p1" || !strings.Contains(body, `"brandName":"Demo Paracetamol 500"`) || !strings.Contains(body, `"strength":"500mg"`) {
		t.Errorf("asked %q, body %s: want the product as JSON", catalog.asked, body)
	}
	if strings.Contains(body, "controlled") {
		t.Errorf("body %s exposes the internal controlled flag", body)
	}
	if rec.Header().Get("Cache-Control") == "" {
		t.Error("no Cache-Control: names should be cacheable")
	}
}

func TestPharmacyLookupIsPublicWithoutTheOwner(t *testing.T) {
	registry := &lookupRegistry{}
	rec := get(t, lookupRouter(&lookupCatalog{}, registry), "/api/v1/pharmacies/ph1")

	if rec.Code != http.StatusOK || registry.asked != "ph1" {
		t.Fatalf("status = %d asked %q, want 200 for ph1: %s", rec.Code, registry.asked, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"name":"Demo Pharmacy Thimphu 01"`) {
		t.Errorf("body %s: want the pharmacy name", body)
	}
	// The contract gives these only to the pharmacy's own staff and admins.
	for _, private := range []string{"owner-secret-id", "ownerUserId", "DEMO-TIM-001", "licenseNumber"} {
		if strings.Contains(body, private) {
			t.Errorf("body %s exposes %q", body, private)
		}
	}
}

func TestLookupsAnswer404ForUnknownIDs(t *testing.T) {
	h := lookupRouter(&lookupCatalog{}, &lookupRegistry{})
	for _, path := range []string{"/api/v1/products/nope", "/api/v1/pharmacies/nope"} {
		if rec := get(t, h, path); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, rec.Code)
		}
	}
}

func TestPharmacyStockStillNeedsAToken(t *testing.T) {
	h := lookupRouter(&lookupCatalog{}, &lookupRegistry{})
	// Only GET of one pharmacy is public; its stock and writes are not.
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/pharmacies/ph1/stock", nil),
		httptest.NewRequest(http.MethodPut, "/api/v1/pharmacies/ph1", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/pharmacies/me/analytics/unmet-demand", nil),
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401 without a token", req.Method, req.URL.Path, rec.Code)
		}
	}
}

func TestLookupServiceErrorsMapToHTTP(t *testing.T) {
	h := NewRouter(Config{CORSOrigins: defaultCORSOrigins}, slog.Default(), WithProducts(downCatalog{}))
	if rec := get(t, h, "/api/v1/products/p1"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when the Catalog is down", rec.Code)
	}
}

type downCatalog struct{}

func (downCatalog) GetProduct(context.Context, *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error) {
	return nil, status.Error(codes.Unavailable, "down")
}
