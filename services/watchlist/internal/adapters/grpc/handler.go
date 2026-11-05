// Package grpc is the inbound adapter: it translates WatchlistService RPCs into
// app calls and domain errors into gRPC status codes. No business rules here.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	watchlistv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
)

// Handler implements watchlistv1.WatchlistServiceServer.
// StreamAlerts is not implemented yet (SCRUM-71); the embedded type answers UNIMPLEMENTED.
type Handler struct {
	watchlistv1.UnimplementedWatchlistServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Subscribe(ctx context.Context, req *watchlistv1.SubscribeRequest) (*watchlistv1.SubscribeResponse, error) {
	if req.GetLocation() == nil {
		return nil, status.Error(codes.InvalidArgument, "location is required")
	}
	at := domain.Point{Lat: req.GetLocation().GetLatitude(), Lng: req.GetLocation().GetLongitude()}
	s, err := h.svc.Subscribe(ctx, req.GetUserId(), req.GetProductId(), at, req.GetRadiusM())
	if err != nil {
		return nil, toStatus(err)
	}
	return &watchlistv1.SubscribeResponse{Subscription: toProto(s)}, nil
}

func (h *Handler) Unsubscribe(ctx context.Context, req *watchlistv1.UnsubscribeRequest) (*watchlistv1.UnsubscribeResponse, error) {
	if err := h.svc.Unsubscribe(ctx, req.GetSubscriptionId(), req.GetUserId()); err != nil {
		return nil, toStatus(err)
	}
	return &watchlistv1.UnsubscribeResponse{}, nil
}

func (h *Handler) ListSubscriptions(ctx context.Context, req *watchlistv1.ListSubscriptionsRequest) (*watchlistv1.ListSubscriptionsResponse, error) {
	subs, err := h.svc.List(ctx, req.GetUserId())
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*watchlistv1.Subscription, 0, len(subs))
	for _, s := range subs {
		out = append(out, toProto(s))
	}
	return &watchlistv1.ListSubscriptionsResponse{Subscriptions: out}, nil
}

func toProto(s domain.Subscription) *watchlistv1.Subscription {
	p := &watchlistv1.Subscription{
		SubscriptionId: s.ID,
		UserId:         s.UserID,
		ProductId:      s.ProductID,
		Location:       &commonv1.GeoPoint{Latitude: s.Location.Lat, Longitude: s.Location.Lng},
		RadiusM:        s.RadiusM,
		CreatedAt:      timestamppb.New(s.CreatedAt),
	}
	if s.Notified() {
		p.NotifiedAt = timestamppb.New(s.NotifiedAt)
	}
	return p
}

// toStatus maps domain validation errors to INVALID_ARGUMENT and ErrNotFound to
// NOT_FOUND; anything else is INTERNAL. The grpcx logging interceptor records
// the full error; callers only see a safe message.
func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrMissingUserID),
		errors.Is(err, domain.ErrMissingProductID),
		errors.Is(err, domain.ErrMissingSubscriptionID),
		errors.Is(err, domain.ErrInvalidLocation),
		errors.Is(err, domain.ErrInvalidRadius):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
