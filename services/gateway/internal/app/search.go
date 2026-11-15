package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	searchv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/search/v1"
	substitutionv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/substitution/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/httpx"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const downstreamTimeout = 2 * time.Second

func searchHandler(client ports.Searcher, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		lat, lng, radius, err := parseLocation(r)
		if err != nil || len([]rune(q)) < 2 || len([]rune(q)) > 100 {
			writeError(w, http.StatusBadRequest, "invalid search parameters")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), downstreamTimeout)
		defer cancel()
		resp, callErr := client.SearchMedicine(ctx, &searchv1.SearchMedicineRequest{
			Query: q, Location: &commonv1.GeoPoint{Latitude: lat, Longitude: lng}, RadiusM: radius,
		})
		if callErr != nil {
			writeDownstreamError(w, logger, r, callErr)
			return
		}
		writeProtoJSON(w, resp)
	}
}

// Suggestion limits, the same as Search's (services/search/internal/domain).
const (
	defaultSuggestions = 8
	maxSuggestions     = 20
)

// autocompleteHandler serves GET /api/v1/search/autocomplete?prefix=&limit=.
func autocompleteHandler(client ports.Suggester, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := strings.TrimSpace(r.URL.Query().Get("prefix"))
		if n := len([]rune(prefix)); n < 2 || n > 100 {
			writeError(w, http.StatusBadRequest, "prefix must be 2 to 100 characters")
			return
		}
		limit, err := queryInt(r, "limit", defaultSuggestions)
		if err != nil || limit < 1 || limit > maxSuggestions {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), downstreamTimeout)
		defer cancel()
		resp, callErr := client.Autocomplete(ctx, &searchv1.AutocompleteRequest{Prefix: prefix, Limit: int32(limit)})
		if callErr != nil {
			writeDownstreamError(w, logger, r, callErr)
			return
		}
		writeProtoJSON(w, resp)
	}
}

func nearbyHandler(client ports.NearbyPharmacyFinder, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lat, lng, radius, err := parseLocation(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid location parameters")
			return
		}
		limit, err := queryInt(r, "limit", 0)
		if err != nil || limit < 0 || limit > 100 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), downstreamTimeout)
		defer cancel()
		resp, callErr := client.FindNearbyPharmacies(ctx, &geov1.FindNearbyPharmaciesRequest{
			Location: &commonv1.GeoPoint{Latitude: lat, Longitude: lng}, RadiusM: radius, Limit: int32(limit),
		})
		if callErr != nil {
			writeDownstreamError(w, logger, r, callErr)
			return
		}
		writeProtoJSON(w, resp)
	}
}

func alternativesHandler(client ports.SubstituteFinder, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeError(w, http.StatusBadRequest, "medicine id is required")
			return
		}
		limit, err := queryInt(r, "limit", 5)
		if err != nil || limit < 0 || limit > 100 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), downstreamTimeout)
		defer cancel()
		resp, callErr := client.GetSubstitutes(ctx, &substitutionv1.GetSubstitutesRequest{ProductId: id, Limit: int32(limit)})
		if callErr != nil {
			writeDownstreamError(w, logger, r, callErr)
			return
		}
		writeProtoJSON(w, resp)
	}
}

func parseLocation(r *http.Request) (float64, float64, int32, error) {
	lat, err := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err != nil || lat < -90 || lat > 90 {
		return 0, 0, 0, fmt.Errorf("latitude")
	}
	lng, err := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err != nil || lng < -180 || lng > 180 {
		return 0, 0, 0, fmt.Errorf("longitude")
	}
	radius, err := queryInt(r, "radius_m", 0)
	if err != nil || radius < 0 || (radius != 0 && (radius < 500 || radius > 50000)) {
		return 0, 0, 0, fmt.Errorf("radius")
	}
	return lat, lng, int32(radius), nil
}

func queryInt(r *http.Request, name string, fallback int) (int, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func writeProtoJSON(w http.ResponseWriter, message interface{ ProtoReflect() protoreflect.Message }) {
	data, err := (protojson.MarshalOptions{UseProtoNames: false}).Marshal(message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func writeDownstreamError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	logger.Error("gRPC request failed", "request_id", httpx.RequestIDFromContext(r.Context()), "error", err.Error())
	code := httpx.StatusCode(err)
	writeError(w, code, errorMessage(code))
}
