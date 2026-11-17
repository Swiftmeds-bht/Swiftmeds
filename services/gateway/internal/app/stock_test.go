package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeInventoryClient struct {
	list *inventoryv1.ListPharmacyStockRequest
	set  *inventoryv1.SetStockRequest
	err  error
}

func (f *fakeInventoryClient) ListPharmacyStock(_ context.Context, req *inventoryv1.ListPharmacyStockRequest) (*inventoryv1.ListPharmacyStockResponse, error) {
	f.list = req
	if f.err != nil {
		return nil, f.err
	}
	return &inventoryv1.ListPharmacyStockResponse{Stock: []*inventoryv1.StockLevel{
		{PharmacyId: req.GetPharmacyId(), ProductId: "p-1", QuantityOnHand: 4, QuantityAvailable: 3, QuantityReserved: 1},
	}}, nil
}

func (f *fakeInventoryClient) SetStock(_ context.Context, req *inventoryv1.SetStockRequest) (*inventoryv1.SetStockResponse, error) {
	f.set = req
	if f.err != nil {
		return nil, f.err
	}
	return &inventoryv1.SetStockResponse{Stock: &inventoryv1.StockLevel{
		PharmacyId: req.GetPharmacyId(), ProductId: req.GetProductId(), QuantityOnHand: req.GetQuantityOnHand(),
	}}, nil
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

var staffOfPharmacy1 = ports.Identity{UserID: "staff-1", PharmacyID: "ph-1", Role: authv1.Role_ROLE_PHARMACY_STAFF}

func stockRequest(method, path, pharmacyID, productID, body string, identity ports.Identity) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	req = withIdentity(req, identity)
	req.SetPathValue("pharmacyId", pharmacyID)
	if productID != "" {
		req.SetPathValue("productId", productID)
	}
	return req
}

func TestStockHandlerListsOwnPharmacyStock(t *testing.T) {
	fake := &fakeInventoryClient{}
	req := stockRequest(http.MethodGet, "/api/v1/pharmacies/ph-1/stock?pageSize=50&pageToken=t1", "ph-1", "", "", staffOfPharmacy1)

	rec := httptest.NewRecorder()
	stockHandler(fake, nil)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if fake.list.GetPharmacyId() != "ph-1" || fake.list.GetPage().GetPageSize() != 50 || fake.list.GetPage().GetPageToken() != "t1" {
		t.Fatalf("unexpected downstream request: %v", fake.list)
	}
	if !strings.Contains(rec.Body.String(), `"quantityAvailable":3`) {
		t.Fatalf("response is missing stock levels: %s", rec.Body.String())
	}
}

func TestStockHandlerRejectsAnotherPharmacy(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			fake := &fakeInventoryClient{}
			productID := ""
			body := ""
			if method == http.MethodPut {
				productID, body = "p-1", `{"quantityOnHand":5}`
			}
			req := stockRequest(method, "/api/v1/pharmacies/ph-2/stock", "ph-2", productID, body, staffOfPharmacy1)

			rec := httptest.NewRecorder()
			stockHandler(fake, nil)(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("got %d, want 403", rec.Code)
			}
			if fake.list != nil || fake.set != nil {
				t.Fatal("downstream call should not be made")
			}
		})
	}
}

func TestStockHandlerRejectsPatients(t *testing.T) {
	fake := &fakeInventoryClient{}
	patient := ports.Identity{UserID: "user-1", PharmacyID: "ph-1", Role: authv1.Role_ROLE_PATIENT}
	req := stockRequest(http.MethodGet, "/api/v1/pharmacies/ph-1/stock", "ph-1", "", "", patient)

	rec := httptest.NewRecorder()
	stockHandler(fake, nil)(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestStockHandlerSetsStock(t *testing.T) {
	fake := &fakeInventoryClient{}
	req := stockRequest(http.MethodPut, "/api/v1/pharmacies/ph-1/stock/p-1", "ph-1", "p-1", `{"quantityOnHand":12}`, staffOfPharmacy1)

	rec := httptest.NewRecorder()
	stockHandler(fake, nil)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	want := &inventoryv1.SetStockRequest{PharmacyId: "ph-1", ProductId: "p-1", QuantityOnHand: 12, UpdatedByUserId: "staff-1"}
	if fake.set.GetPharmacyId() != want.PharmacyId || fake.set.GetProductId() != want.ProductId ||
		fake.set.GetQuantityOnHand() != want.QuantityOnHand || fake.set.GetUpdatedByUserId() != want.UpdatedByUserId {
		t.Fatalf("got %v, want %v", fake.set, want)
	}
	if !strings.Contains(rec.Body.String(), `"quantityOnHand":12`) {
		t.Fatalf("response is missing the new quantity: %s", rec.Body.String())
	}
}

func TestStockHandlerSetRejectsInvalidQuantity(t *testing.T) {
	for name, body := range map[string]string{
		"missing":  `{}`,
		"negative": `{"quantityOnHand":-1}`,
		"fraction": `{"quantityOnHand":1.5}`,
		"too big":  `{"quantityOnHand":2147483648}`,
		"string":   `{"quantityOnHand":"3"}`,
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeInventoryClient{}
			req := stockRequest(http.MethodPut, "/api/v1/pharmacies/ph-1/stock/p-1", "ph-1", "p-1", body, staffOfPharmacy1)

			rec := httptest.NewRecorder()
			stockHandler(fake, nil)(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400", rec.Code)
			}
			if fake.set != nil {
				t.Fatal("downstream call should not be made")
			}
		})
	}
}

func TestStockHandlerMapsDownstreamErrors(t *testing.T) {
	fake := &fakeInventoryClient{err: status.Error(codes.NotFound, "stock not found")}
	req := stockRequest(http.MethodPut, "/api/v1/pharmacies/ph-1/stock/p-9", "ph-1", "p-9", `{"quantityOnHand":1}`, staffOfPharmacy1)

	rec := httptest.NewRecorder()
	stockHandler(fake, testLogger())(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

func TestStockHandlerSetBelowReservedIsBadRequest(t *testing.T) {
	fake := &fakeInventoryClient{err: status.Error(codes.FailedPrecondition, "on hand below reserved")}
	req := stockRequest(http.MethodPut, "/api/v1/pharmacies/ph-1/stock/p-1", "ph-1", "p-1", `{"quantityOnHand":0}`, staffOfPharmacy1)

	rec := httptest.NewRecorder()
	stockHandler(fake, testLogger())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "reserved by active holds") {
		t.Fatalf("response should explain the reserved quantity: %s", rec.Body.String())
	}
}

func TestStockHandlerSetIsStaffOnly(t *testing.T) {
	fake := &fakeInventoryClient{}
	admin := ports.Identity{UserID: "admin-1", PharmacyID: "ph-1", Role: authv1.Role_ROLE_ADMIN}

	list := stockRequest(http.MethodGet, "/api/v1/pharmacies/ph-1/stock", "ph-1", "", "", admin)
	rec := httptest.NewRecorder()
	stockHandler(fake, nil)(rec, list)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin list: got %d, want 200", rec.Code)
	}

	set := stockRequest(http.MethodPut, "/api/v1/pharmacies/ph-1/stock/p-1", "ph-1", "p-1", `{"quantityOnHand":3}`, admin)
	rec = httptest.NewRecorder()
	stockHandler(fake, nil)(rec, set)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin set: got %d, want 403", rec.Code)
	}
	if fake.set != nil {
		t.Fatal("downstream call should not be made")
	}
}
