package app

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	searchv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/search/v1"
	substitutionv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/substitution/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeSearcher struct {
	response *searchv1.SearchMedicineResponse
	err      error
	request  *searchv1.SearchMedicineRequest
}

func (f *fakeSearcher) SearchMedicine(_ context.Context, req *searchv1.SearchMedicineRequest) (*searchv1.SearchMedicineResponse, error) {
	f.request = req
	return f.response, f.err
}

type fakeGeo struct {
	response *geov1.FindNearbyPharmaciesResponse
	err      error
	request  *geov1.FindNearbyPharmaciesRequest
}

func (f *fakeGeo) FindNearbyPharmacies(_ context.Context, req *geov1.FindNearbyPharmaciesRequest) (*geov1.FindNearbyPharmaciesResponse, error) {
	f.request = req
	return f.response, f.err
}

type fakeSubstitution struct {
	response *substitutionv1.GetSubstitutesResponse
	err      error
	request  *substitutionv1.GetSubstitutesRequest
}

func (f *fakeSubstitution) GetSubstitutes(_ context.Context, req *substitutionv1.GetSubstitutesRequest) (*substitutionv1.GetSubstitutesResponse, error) {
	f.request = req
	return f.response, f.err
}

func TestSearchRoutesSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	t.Run("search", func(t *testing.T) {
		fake := &fakeSearcher{response: &searchv1.SearchMedicineResponse{SearchId: "s-1"}}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=napa&lat=27.47&lng=89.64&radius_m=1000", nil)
		rec := httptest.NewRecorder()
		searchHandler(fake, logger).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"searchId":"s-1"`) {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		if fake.request.GetLocation().GetLatitude() != 27.47 || fake.request.GetRadiusM() != 1000 {
			t.Fatalf("request was not mapped: %v", fake.request)
		}
	})
	t.Run("nearby", func(t *testing.T) {
		fake := &fakeGeo{response: &geov1.FindNearbyPharmaciesResponse{}}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pharmacies/nearby?lat=27.47&lng=89.64&radius_m=1000", nil)
		rec := httptest.NewRecorder()
		nearbyHandler(fake, logger).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "{}" {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("alternatives", func(t *testing.T) {
		fake := &fakeSubstitution{response: &substitutionv1.GetSubstitutesResponse{MoleculeId: "m-1"}}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/medicines/p-1/alternatives?limit=3", nil)
		req.SetPathValue("id", "p-1")
		rec := httptest.NewRecorder()
		alternativesHandler(fake, logger).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"moleculeId":"m-1"`) {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		if fake.request.GetProductId() != "p-1" || fake.request.GetLimit() != 3 {
			t.Fatalf("request was not mapped: %v", fake.request)
		}
	})
}

func TestSearchRoutesMapGRPCErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	for _, code := range []codes.Code{codes.NotFound, codes.InvalidArgument, codes.Unavailable} {
		t.Run(code.String(), func(t *testing.T) {
			fake := &fakeSearcher{err: status.Error(code, "downstream detail")}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=napa&lat=27.47&lng=89.64", nil)
			rec := httptest.NewRecorder()
			searchHandler(fake, logger).ServeHTTP(rec, req)
			want := map[codes.Code]int{
				codes.NotFound:        http.StatusNotFound,
				codes.InvalidArgument: http.StatusBadRequest,
				codes.Unavailable:     http.StatusServiceUnavailable,
			}[code]
			if rec.Code != want || strings.Contains(rec.Body.String(), "downstream detail") {
				t.Fatalf("got %d %s, want %d without internal detail", rec.Code, rec.Body.String(), want)
			}
		})
	}
}

func TestOtherRoutesMapGRPCErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	for _, code := range []codes.Code{codes.NotFound, codes.InvalidArgument, codes.Unavailable} {
		want := map[codes.Code]int{
			codes.NotFound:        http.StatusNotFound,
			codes.InvalidArgument: http.StatusBadRequest,
			codes.Unavailable:     http.StatusServiceUnavailable,
		}[code]
		t.Run("nearby/"+code.String(), func(t *testing.T) {
			fake := &fakeGeo{err: status.Error(code, "downstream detail")}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/pharmacies/nearby?lat=27&lng=89", nil)
			rec := httptest.NewRecorder()
			nearbyHandler(fake, logger).ServeHTTP(rec, req)
			if rec.Code != want {
				t.Fatalf("got %d, want %d", rec.Code, want)
			}
		})
		t.Run("alternatives/"+code.String(), func(t *testing.T) {
			fake := &fakeSubstitution{err: status.Error(code, "downstream detail")}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/medicines/p-1/alternatives", nil)
			req.SetPathValue("id", "p-1")
			rec := httptest.NewRecorder()
			alternativesHandler(fake, logger).ServeHTTP(rec, req)
			if rec.Code != want {
				t.Fatalf("got %d, want %d", rec.Code, want)
			}
		})
	}
}

func TestSearchValidation(t *testing.T) {
	fake := &fakeSearcher{response: &searchv1.SearchMedicineResponse{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=x&lat=27&lng=89", nil)
	rec := httptest.NewRecorder()
	searchHandler(fake, slog.Default()).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}
