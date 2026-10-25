// Package grpc is the inbound adapter: GeoService RPCs <-> app calls, domain
// errors -> gRPC status codes. No business rules here.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/domain"
)

type Handler struct {
	geov1.UnimplementedGeoServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) FindNearbyPharmacies(ctx context.Context, req *geov1.FindNearbyPharmaciesRequest) (*geov1.FindNearbyPharmaciesResponse, error) {
	if req.GetLocation() == nil {
		return nil, status.Error(codes.InvalidArgument, "location is required")
	}
	results, err := h.svc.FindNearby(ctx, toPoint(req.GetLocation()), req.GetRadiusM(), req.GetLimit())
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*geov1.NearbyPharmacy, 0, len(results))
	for _, r := range results {
		out = append(out, &geov1.NearbyPharmacy{PharmacyId: r.PharmacyID, DistanceM: r.DistanceMetres})
	}
	return &geov1.FindNearbyPharmaciesResponse{Pharmacies: out}, nil
}

func (h *Handler) UpsertPharmacyLocation(ctx context.Context, req *geov1.UpsertPharmacyLocationRequest) (*geov1.UpsertPharmacyLocationResponse, error) {
	if req.GetLocation() == nil {
		return nil, status.Error(codes.InvalidArgument, "location is required")
	}
	loc := domain.Location{PharmacyID: req.GetPharmacyId(), Point: toPoint(req.GetLocation())}
	if err := h.svc.UpsertLocation(ctx, loc); err != nil {
		return nil, toStatus(err)
	}
	return &geov1.UpsertPharmacyLocationResponse{}, nil
}

func (h *Handler) RemovePharmacyLocation(ctx context.Context, req *geov1.RemovePharmacyLocationRequest) (*geov1.RemovePharmacyLocationResponse, error) {
	if err := h.svc.RemoveLocation(ctx, req.GetPharmacyId()); err != nil {
		return nil, toStatus(err)
	}
	return &geov1.RemovePharmacyLocationResponse{}, nil
}

func toPoint(p *commonv1.GeoPoint) domain.Point {
	return domain.Point{Lat: p.GetLatitude(), Lng: p.GetLongitude()}
}

// toStatus maps validation errors to INVALID_ARGUMENT; anything else is INTERNAL.
func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidLocation),
		errors.Is(err, domain.ErrInvalidRadius),
		errors.Is(err, domain.ErrInvalidLimit),
		errors.Is(err, domain.ErrMissingPharmacyID):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
