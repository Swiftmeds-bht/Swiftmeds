package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

type fakePricing struct {
	set    *pricingv1.SetPricesRequest
	get    *pricingv1.BatchGetPricesRequest
	setErr error
}

func (f *fakePricing) SetPrices(_ context.Context, req *pricingv1.SetPricesRequest) (*pricingv1.SetPricesResponse, error) {
	f.set = req
	if f.setErr != nil {
		return nil, f.setErr
	}
	out := &pricingv1.SetPricesResponse{}
	for _, u := range req.GetUpdates() {
		out.Prices = append(out.Prices, &pricingv1.Price{
			Key:           &commonv1.StockKey{PharmacyId: req.GetPharmacyId(), ProductId: u.GetProductId()},
			PriceChhertum: u.GetPriceChhertum(),
			UpdatedAt:     timestamppb.New(time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)),
		})
	}
	return out, nil
}

func (f *fakePricing) BatchGetPrices(_ context.Context, req *pricingv1.BatchGetPricesRequest) (*pricingv1.BatchGetPricesResponse, error) {
	f.get = req
	return &pricingv1.BatchGetPricesResponse{Prices: []*pricingv1.Price{{
		Key:           &commonv1.StockKey{PharmacyId: "ph-1", ProductId: "p-1"},
		PriceChhertum: 4500,
	}}}, nil
}

func pricesRouter(identity ports.Identity, pricing *fakePricing) http.Handler {
	return pharmacyRouterWith(identity, WithPricing(pricing))
}

// pharmacyRouterWith is pharmacyRouter for any caller.
func pharmacyRouterWith(identity ports.Identity, opts ...RouterOption) http.Handler {
	all := append([]RouterOption{WithAuth(staticAuthenticator{identity: identity}), WithPharmacyRegistry(&fakePharmacyRegistryClient{})}, opts...)
	return NewRouter(Config{}, testLogger(), all...)
}

func TestSetPricesForOwnPharmacy(t *testing.T) {
	pricing := &fakePricing{}
	rec := serve(pricesRouter(staffOfPharmacy1, pricing), http.MethodPut, "/api/v1/pharmacies/ph-1/prices",
		`{"updates":[{"productId":"p-1","priceChhertum":"4500"},{"productId":"p-2","priceChhertum":1250}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if pricing.set.GetPharmacyId() != "ph-1" || len(pricing.set.GetUpdates()) != 2 || pricing.set.GetUpdates()[1].GetPriceChhertum() != 1250 {
		t.Fatalf("pricing got %v", pricing.set)
	}
	var body struct {
		Prices []map[string]string `json:"prices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"pharmacyId": "ph-1", "productId": "p-1", "priceChhertum": "4500", "updatedAt": "2026-11-26T10:00:00Z"}
	if len(body.Prices) != 2 || body.Prices[0]["pharmacyId"] != want["pharmacyId"] || body.Prices[0]["priceChhertum"] != "4500" || body.Prices[0]["updatedAt"] != want["updatedAt"] {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestSetPricesValidation(t *testing.T) {
	cases := map[string]string{
		"no updates":       `{"updates":[]}`,
		"zero price":       `{"updates":[{"productId":"p-1","priceChhertum":"0"}]}`,
		"negative":         `{"updates":[{"productId":"p-1","priceChhertum":-5}]}`,
		"decimal":          `{"updates":[{"productId":"p-1","priceChhertum":"45.5"}]}`,
		"too large":        `{"updates":[{"productId":"p-1","priceChhertum":"100000001"}]}`,
		"missing price":    `{"updates":[{"productId":"p-1"}]}`,
		"missing product":  `{"updates":[{"priceChhertum":"100"}]}`,
		"duplicate":        `{"updates":[{"productId":"p-1","priceChhertum":"100"},{"productId":"p-1","priceChhertum":"200"}]}`,
		"unknown field":    `{"updates":[{"productId":"p-1","priceChhertum":"100","pharmacyId":"ph-2"}]}`,
		"pharmacy in body": `{"pharmacyId":"ph-2","updates":[{"productId":"p-1","priceChhertum":"100"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			pricing := &fakePricing{}
			rec := serve(pricesRouter(staffOfPharmacy1, pricing), http.MethodPut, "/api/v1/pharmacies/ph-1/prices", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body.String())
			}
			if pricing.set != nil {
				t.Fatal("an invalid request reached Pricing")
			}
		})
	}
	tooMany := `{"updates":[` + strings.Repeat(`{"productId":"p","priceChhertum":"1"},`, maxPriceUpdates) + `{"productId":"q","priceChhertum":"1"}]}`
	if rec := serve(pricesRouter(staffOfPharmacy1, &fakePricing{}), http.MethodPut, "/api/v1/pharmacies/ph-1/prices", tooMany); rec.Code != http.StatusBadRequest {
		t.Fatalf("101 updates: got %d", rec.Code)
	}
}

func TestSetPricesIsOwnPharmacyStaffOnly(t *testing.T) {
	body := `{"updates":[{"productId":"p-1","priceChhertum":"100"}]}`
	admin := ports.Identity{UserID: "a-1", Role: authv1.Role_ROLE_ADMIN, PharmacyID: "ph-1"}
	patient := ports.Identity{UserID: "u-1", Role: authv1.Role_ROLE_PATIENT}
	for name, tc := range map[string]struct {
		who  ports.Identity
		path string
	}{
		"another pharmacy": {staffOfPharmacy1, "/api/v1/pharmacies/ph-2/prices"},
		"admin":            {admin, "/api/v1/pharmacies/ph-1/prices"},
		"patient":          {patient, "/api/v1/pharmacies/ph-1/prices"},
	} {
		t.Run(name, func(t *testing.T) {
			pricing := &fakePricing{}
			if rec := serve(pricesRouter(tc.who, pricing), http.MethodPut, tc.path, body); rec.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 403", rec.Code)
			}
			if pricing.set != nil {
				t.Fatal("a refused request reached Pricing")
			}
		})
	}
}

func TestSetPricesPassesPricingValidation(t *testing.T) {
	pricing := &fakePricing{setErr: status.Error(codes.InvalidArgument, "price must be greater than zero")}
	rec := serve(pricesRouter(staffOfPharmacy1, pricing), http.MethodPut, "/api/v1/pharmacies/ph-1/prices", `{"updates":[{"productId":"p-1","priceChhertum":"100"}]}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "greater than zero") {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPricesForOwnPharmacy(t *testing.T) {
	pricing := &fakePricing{}
	rec := serve(pricesRouter(staffOfPharmacy1, pricing), http.MethodGet, "/api/v1/pharmacies/ph-1/prices?productId=p-1&productId=p-2", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"priceChhertum":"4500"`) {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	keys := pricing.get.GetKeys()
	if len(keys) != 2 || keys[0].GetPharmacyId() != "ph-1" || keys[1].GetProductId() != "p-2" {
		t.Fatalf("pricing got %v", pricing.get)
	}
	if rec := serve(pricesRouter(staffOfPharmacy1, pricing), http.MethodGet, "/api/v1/pharmacies/ph-1/prices", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("no productId: got %d", rec.Code)
	}
	if rec := serve(pricesRouter(staffOfPharmacy1, pricing), http.MethodGet, "/api/v1/pharmacies/ph-2/prices?productId=p-1", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("another pharmacy: got %d", rec.Code)
	}
}

func TestRouterWithoutPricingAnswers404ForPrices(t *testing.T) {
	rec := serve(pharmacyRouterWith(staffOfPharmacy1), http.MethodPut, "/api/v1/pharmacies/ph-1/prices", `{"updates":[]}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}
