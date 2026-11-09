package app

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

func TestHoldHandlerAllowsOwnerToReadPickupCode(t *testing.T) {
	fake := &fakeHoldsClient{response: &holdsv1.Hold{
		HoldId: "h-1", UserId: "user-1", PharmacyId: "ph-1", PickupCode: "482913",
	}}
	req := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/holds/h-1", nil), ports.Identity{
		UserID: "user-1", Role: authv1.Role_ROLE_PATIENT,
	})
	req.SetPathValue("holdId", "h-1")

	rec := httptest.NewRecorder()
	holdsHandler(fake, nil)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "482913") {
		t.Fatalf("owner response omitted pickup code: %q", rec.Body.String())
	}
}

func TestHoldHandlerRejectsPharmacyFromAnotherPharmacy(t *testing.T) {
	fake := &fakeHoldsClient{response: &holdsv1.Hold{
		HoldId: "h-1", PharmacyId: "ph-2", PickupCode: "482913",
	}}
	req := withIdentity(httptest.NewRequest(http.MethodGet, "/api/v1/holds/h-1", nil), ports.Identity{
		PharmacyID: "ph-1", Role: authv1.Role_ROLE_PHARMACY_STAFF,
	})
	req.SetPathValue("holdId", "h-1")

	rec := httptest.NewRecorder()
	holdsHandler(fake, nil)(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestHoldHandlerCreateRejectsOutOfRangeQuantity(t *testing.T) {
	for _, quantity := range []int{0, 6} {
		t.Run(strconv.Itoa(quantity), func(t *testing.T) {
			fake := &fakeHoldsClient{}
			req := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds",
				strings.NewReader(`{"pharmacyId":"ph-1","productId":"p-1","quantity":`+strconv.Itoa(quantity)+`}`)),
				ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT})

			rec := httptest.NewRecorder()
			holdsHandler(fake, nil)(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400", rec.Code)
			}
			if fake.create != nil {
				t.Fatal("downstream create call should not be made")
			}
		})
	}
}

func TestHoldHandlerCancelAllowsEmptyBody(t *testing.T) {
	fake := &fakeHoldsClient{response: &holdsv1.Hold{HoldId: "h-1"}}
	req := withIdentity(httptest.NewRequest(http.MethodPost, "/api/v1/holds/h-1/cancel", nil), ports.Identity{
		UserID: "user-1", Role: authv1.Role_ROLE_PATIENT,
	})
	req.SetPathValue("holdId", "h-1")

	rec := httptest.NewRecorder()
	holdsHandler(fake, nil)(rec, req)

	if rec.Code != http.StatusOK || fake.cancel == nil {
		t.Fatalf("got %d and request %v, want 200 with cancel request", rec.Code, fake.cancel)
	}
	if fake.cancel.GetReason() != "" {
		t.Fatalf("got reason %q, want empty reason", fake.cancel.GetReason())
	}
}

func TestHoldHandlerRejectsInvalidRequestBodies(t *testing.T) {
	tests := []struct {
		name   string
		req    *http.Request
		holdID bool
	}{
		{
			name: "create without body",
			req: withIdentity(
				httptest.NewRequest(http.MethodPost, "/api/v1/holds", nil),
				ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT},
			),
		},
		{
			name: "cancel with malformed JSON",
			req: withIdentity(
				httptest.NewRequest(http.MethodPost, "/api/v1/holds/h-1/cancel", strings.NewReader("{")),
				ports.Identity{UserID: "user-1", Role: authv1.Role_ROLE_PATIENT},
			),
			holdID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.holdID {
				tt.req.SetPathValue("holdId", "h-1")
			}
			rec := httptest.NewRecorder()
			holdsHandler(&fakeHoldsClient{}, nil)(rec, tt.req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400", rec.Code)
			}
		})
	}
}
