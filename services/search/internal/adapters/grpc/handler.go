// Package grpc is Search's inbound adapter: SearchService <-> app, errors ->
// gRPC status codes. No business rules here.
package grpc

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	searchv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/search/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/domain"
)

// DegradedHeader carries the optional services that did not answer (comma
// separated) in the response header, so the gateway can tell partial results
// apart without a contract change.
const DegradedHeader = "x-search-degraded"

type Handler struct {
	searchv1.UnimplementedSearchServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) SearchMedicine(ctx context.Context, req *searchv1.SearchMedicineRequest) (*searchv1.SearchMedicineResponse, error) {
	if req.GetLocation() == nil {
		return nil, status.Error(codes.InvalidArgument, "location is required")
	}
	center := domain.Point{Lat: req.GetLocation().GetLatitude(), Lng: req.GetLocation().GetLongitude()}
	q, err := domain.NewQuery(req.GetQuery(), center, req.GetRadiusM(), req.GetMaxOffers())
	if err != nil {
		return nil, toStatus(err)
	}
	res, err := h.svc.Search(ctx, q)
	if err != nil {
		return nil, toStatus(err)
	}
	if len(res.Degraded) > 0 {
		// Best effort: a failure to set a header must not fail the search.
		_ = grpc.SetHeader(ctx, metadata.Pairs(DegradedHeader, strings.Join(res.Degraded, ",")))
	}
	return &searchv1.SearchMedicineResponse{
		SearchId:    res.SearchID,
		Results:     toProtoResults(res.Results),
		Substitutes: toProtoResults(res.Substitutes),
	}, nil
}

func (h *Handler) Autocomplete(ctx context.Context, req *searchv1.AutocompleteRequest) (*searchv1.AutocompleteResponse, error) {
	q, err := domain.NewSuggestQuery(req.GetPrefix(), req.GetLimit())
	if err != nil {
		return nil, toStatus(err)
	}
	products, err := h.svc.Autocomplete(ctx, q)
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*searchv1.Suggestion, 0, len(products))
	for _, p := range products {
		out = append(out, &searchv1.Suggestion{ProductId: p.ID, DisplayName: p.DisplayName()})
	}
	return &searchv1.AutocompleteResponse{Suggestions: out}, nil
}

func toProtoResults(rs []domain.ProductResult) []*searchv1.ProductResult {
	out := make([]*searchv1.ProductResult, 0, len(rs))
	for _, r := range rs {
		offers := make([]*searchv1.PharmacyOffer, 0, len(r.Offers))
		for _, o := range r.Offers {
			po := &searchv1.PharmacyOffer{
				PharmacyId:        o.PharmacyID,
				PharmacyName:      o.PharmacyName,
				Address:           o.Address,
				DistanceM:         o.DistanceM,
				QuantityAvailable: o.Quantity,
				PriceChhertum:     o.PriceChhertum,
				FreshnessScore:    o.Freshness,
				TrustScore:        o.Trust,
				RankScore:         o.RankScore,
			}
			if o.Location != nil {
				po.Location = &commonv1.GeoPoint{Latitude: o.Location.Lat, Longitude: o.Location.Lng}
			}
			if !o.ConfirmedAt.IsZero() {
				po.StockConfirmedAt = timestamppb.New(o.ConfirmedAt)
			}
			offers = append(offers, po)
		}
		out = append(out, &searchv1.ProductResult{
			ProductId: r.ID, BrandName: r.BrandName, GenericName: r.GenericName, Strength: r.Strength, Offers: offers,
		})
	}
	return out
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidQuery), errors.Is(err, domain.ErrInvalidLocation),
		errors.Is(err, domain.ErrInvalidRadius), errors.Is(err, domain.ErrInvalidMaxOffer),
		errors.Is(err, domain.ErrInvalidLimit):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "search took too long")
	case errors.Is(err, app.ErrUpstream):
		return status.Error(codes.Unavailable, "search is temporarily unavailable")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
