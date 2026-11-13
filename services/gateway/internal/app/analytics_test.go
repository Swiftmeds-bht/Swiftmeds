package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	analyticsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/analytics/v1"
	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

type fakeAnalyticsClient struct {
	request  *analyticsv1.GetUnmetDemandRequest
	response *analyticsv1.GetUnmetDemandResponse
}

func (f *fakeAnalyticsClient) GetUnmetDemand(
	_ context.Context,
	req *analyticsv1.GetUnmetDemandRequest,
) (*analyticsv1.GetUnmetDemandResponse, error) {
	f.request = req
	return f.response, nil
}

type fakePharmacyRegistryClient struct {
	request  *pharmacyregistryv1.GetPharmacyRequest
	response *pharmacyregistryv1.GetPharmacyResponse
}

func (f *fakePharmacyRegistryClient) GetPharmacy(
	_ context.Context,
	req *pharmacyregistryv1.GetPharmacyRequest,
) (*pharmacyregistryv1.GetPharmacyResponse, error) {
	f.request = req
	return f.response, nil
}

func TestUnmetDemandHandlerUsesPharmacyAreaAndLastSevenDays(t *testing.T) {
	registry := &fakePharmacyRegistryClient{
		response: &pharmacyregistryv1.GetPharmacyResponse{
			Pharmacy: &pharmacyregistryv1.Pharmacy{Dzongkhag: "  Paro  "},
		},
	}
	analytics := &fakeAnalyticsClient{
		response: &analyticsv1.GetUnmetDemandResponse{
			Items: []*analyticsv1.UnmetDemandItem{
				{ProductId: "product-1", ProductName: "Demo medicine", UnmetCount: 12},
			},
		},
	}

	req := withIdentity(
		httptest.NewRequest(http.MethodGet, "/api/v1/pharmacies/me/analytics/unmet-demand", nil),
		ports.Identity{PharmacyID: "pharmacy-1", Role: authv1.Role_ROLE_PHARMACY_STAFF},
	)
	rec := httptest.NewRecorder()

	unmetDemandHandler(analytics, registry, nil)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if registry.request == nil || registry.request.GetPharmacyId() != "pharmacy-1" {
		t.Fatalf("registry request = %v, want pharmacy-1", registry.request)
	}
	if analytics.request == nil {
		t.Fatal("Analytics was not called")
	}
	if analytics.request.GetDzongkhag() != "Paro" || analytics.request.GetLimit() != 10 {
		t.Fatalf("Analytics request = %v, want dzongkhag Paro and limit 10", analytics.request)
	}

	timeRange := analytics.request.GetRange()
	if timeRange == nil || timeRange.GetFrom() == nil || timeRange.GetTo() == nil {
		t.Fatalf("Analytics request has no complete time range: %v", timeRange)
	}
	if got := timeRange.GetTo().AsTime().Sub(timeRange.GetFrom().AsTime()); got != 7*24*time.Hour {
		t.Fatalf("range duration = %v, want 7 days", got)
	}
	if !strings.Contains(rec.Body.String(), "Demo medicine") {
		t.Fatalf("response omitted unmet-demand item: %s", rec.Body.String())
	}
}

func TestUnmetDemandHandlerRejectsPatient(t *testing.T) {
	registry := &fakePharmacyRegistryClient{}
	analytics := &fakeAnalyticsClient{}
	req := withIdentity(
		httptest.NewRequest(http.MethodGet, "/api/v1/pharmacies/me/analytics/unmet-demand", nil),
		ports.Identity{UserID: "patient-1", Role: authv1.Role_ROLE_PATIENT},
	)
	rec := httptest.NewRecorder()

	unmetDemandHandler(analytics, registry, nil)(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if registry.request != nil || analytics.request != nil {
		t.Fatal("downstream service was called for a patient")
	}
}

func TestUnmetDemandHandlerRejectsMissingDzongkhag(t *testing.T) {
	registry := &fakePharmacyRegistryClient{
		response: &pharmacyregistryv1.GetPharmacyResponse{
			Pharmacy: &pharmacyregistryv1.Pharmacy{Dzongkhag: "  "},
		},
	}
	analytics := &fakeAnalyticsClient{}
	req := withIdentity(
		httptest.NewRequest(http.MethodGet, "/api/v1/pharmacies/me/analytics/unmet-demand", nil),
		ports.Identity{PharmacyID: "pharmacy-1", Role: authv1.Role_ROLE_PHARMACY_STAFF},
	)
	rec := httptest.NewRecorder()

	unmetDemandHandler(analytics, registry, nil)(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if analytics.request != nil {
		t.Fatal("Analytics was called without a dzongkhag")
	}
}
