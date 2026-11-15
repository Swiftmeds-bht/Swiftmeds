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

// fakeSearchService answers both search and autocomplete, as the Search client does.
type fakeSearchService struct {
	fakeSearcher
	suggestions *searchv1.AutocompleteResponse
	suggestErr  error
	suggestReq  *searchv1.AutocompleteRequest
}

func (f *fakeSearchService) Autocomplete(_ context.Context, req *searchv1.AutocompleteRequest) (*searchv1.AutocompleteResponse, error) {
	f.suggestReq = req
	return f.suggestions, f.suggestErr
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

func TestAutocompleteRoute(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	napa := &searchv1.AutocompleteResponse{Suggestions: []*searchv1.Suggestion{
		{ProductId: "p-1", DisplayName: "Napa 500mg (Paracetamol)"},
	}}

	t.Run("public, mapped and shaped like the spec", func(t *testing.T) {
		fake := &fakeSearchService{suggestions: napa}
		// No WithAuth and no bearer token: the route must still answer.
		router := NewRouter(Config{CORSOrigins: defaultCORSOrigins}, logger, WithSearch(fake))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search/autocomplete?prefix=%20na%20&limit=5", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		want := `{"suggestions":[{"productId":"p-1","displayName":"Napa 500mg (Paracetamol)"}]}`
		if rec.Code != http.StatusOK || strings.ReplaceAll(rec.Body.String(), " ", "") != strings.ReplaceAll(want, " ", "") {
			t.Fatalf("got %d %s, want 200 %s", rec.Code, rec.Body.String(), want)
		}
		if fake.suggestReq.GetPrefix() != "na" || fake.suggestReq.GetLimit() != 5 {
			t.Fatalf("request was not mapped: %v", fake.suggestReq)
		}
	})

	t.Run("default limit", func(t *testing.T) {
		fake := &fakeSearchService{suggestions: &searchv1.AutocompleteResponse{}}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search/autocomplete?prefix=na", nil)
		rec := httptest.NewRecorder()
		autocompleteHandler(fake, logger).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || fake.suggestReq.GetLimit() != defaultSuggestions {
			t.Fatalf("got %d, limit %d", rec.Code, fake.suggestReq.GetLimit())
		}
	})

	for name, query := range map[string]string{
		"no prefix":        "",
		"short prefix":     "prefix=n",
		"blank prefix":     "prefix=%20%20%20",
		"long prefix":      "prefix=" + strings.Repeat("n", 101),
		"zero limit":       "prefix=na&limit=0",
		"limit over max":   "prefix=na&limit=21",
		"non-number limit": "prefix=na&limit=many",
	} {
		t.Run("400/"+name, func(t *testing.T) {
			fake := &fakeSearchService{suggestions: napa}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search/autocomplete?"+query, nil)
			rec := httptest.NewRecorder()
			autocompleteHandler(fake, logger).ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || fake.suggestReq != nil {
				t.Fatalf("got %d (called: %v), want 400 without a Search call", rec.Code, fake.suggestReq != nil)
			}
		})
	}

	t.Run("downstream error hides detail", func(t *testing.T) {
		fake := &fakeSearchService{suggestErr: status.Error(codes.Unavailable, "catalog down: dial tcp")}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search/autocomplete?prefix=na", nil)
		rec := httptest.NewRecorder()
		autocompleteHandler(fake, logger).ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "dial tcp") {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("absent when the client cannot suggest", func(t *testing.T) {
		router := NewRouter(Config{CORSOrigins: defaultCORSOrigins}, logger, WithSearch(&fakeSearcher{}))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/search/autocomplete?prefix=na", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404", rec.Code)
		}
	})
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
