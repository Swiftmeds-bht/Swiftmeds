package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

type staticAuthenticator struct{ identity ports.Identity }

func (a staticAuthenticator) ValidateToken(context.Context, string) (ports.Identity, error) {
	return a.identity, nil
}

// pharmacyRouter builds the real router with every pharmacy dashboard route,
// so route patterns that clash with each other fail here instead of at startup.
func pharmacyRouter(t *testing.T, opts ...RouterOption) http.Handler {
	t.Helper()
	all := append([]RouterOption{
		WithAuth(staticAuthenticator{identity: staffOfPharmacy1}),
		WithHolds(&fakeHoldsClient{}),
		WithAnalytics(&fakeAnalyticsClient{}),
		WithPharmacyRegistry(&fakePharmacyRegistryClient{}),
	}, opts...)
	return NewRouter(Config{}, testLogger(), all...)
}

func serve(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRouterServesDashboardStockRoutes(t *testing.T) {
	inventory := &fakeInventoryClient{}
	router := pharmacyRouter(t, WithInventory(inventory))

	if rec := serve(router, http.MethodGet, "/api/v1/pharmacies/ph-1/stock?pageSize=50", ""); rec.Code != http.StatusOK {
		t.Fatalf("list stock: got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec := serve(router, http.MethodPut, "/api/v1/pharmacies/ph-1/stock/p-1", `{"quantityOnHand":7}`); rec.Code != http.StatusOK {
		t.Fatalf("set stock: got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if inventory.set.GetQuantityOnHand() != 7 {
		t.Fatalf("set stock did not reach Inventory: %v", inventory.set)
	}
}

func TestRouterWithoutInventoryAnswers404ForStock(t *testing.T) {
	router := pharmacyRouter(t)

	if rec := serve(router, http.MethodGet, "/api/v1/pharmacies/ph-1/stock", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}
