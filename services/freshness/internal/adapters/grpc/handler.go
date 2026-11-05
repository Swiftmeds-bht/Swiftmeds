// Package grpc is the inbound adapter: it translates FreshnessService RPCs into
// app calls and domain errors into gRPC status codes. No business rules here.
package grpc

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	freshnessv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/freshness/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
)

// Handler implements freshnessv1.FreshnessServiceServer.
type Handler struct {
	freshnessv1.UnimplementedFreshnessServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RecordConfirmation(ctx context.Context, req *freshnessv1.RecordConfirmationRequest) (*freshnessv1.RecordConfirmationResponse, error) {
	rating, err := h.svc.RecordConfirmation(ctx,
		req.GetKey().GetPharmacyId(), req.GetKey().GetProductId(), toTime(req.GetConfirmedAt()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &freshnessv1.RecordConfirmationResponse{Freshness: toProto(rating)}, nil
}

// GetFreshness is the batch lookup. Keys that were never confirmed are left out.
func (h *Handler) GetFreshness(ctx context.Context, req *freshnessv1.GetFreshnessRequest) (*freshnessv1.GetFreshnessResponse, error) {
	keys := make([]domain.Key, 0, len(req.GetKeys()))
	for _, k := range req.GetKeys() {
		keys = append(keys, domain.Key{PharmacyID: k.GetPharmacyId(), ProductID: k.GetProductId()})
	}
	ratings, err := h.svc.BatchGetConfidence(ctx, keys)
	if err != nil {
		return nil, toStatus(err)
	}
	scores := make([]*freshnessv1.FreshnessScore, 0, len(ratings))
	for _, r := range ratings {
		scores = append(scores, toProto(r))
	}
	return &freshnessv1.GetFreshnessResponse{Scores: scores}, nil
}

// ApplyDecay is the RPC the freshness-decay Worker calls on a schedule.
func (h *Handler) ApplyDecay(ctx context.Context, req *freshnessv1.ApplyDecayRequest) (*freshnessv1.ApplyDecayResponse, error) {
	updated, err := h.svc.Decay(ctx, toTime(req.GetAsOf()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &freshnessv1.ApplyDecayResponse{UpdatedCount: int32(updated)}, nil
}

// toTime turns a missing timestamp into the zero time. (A nil timestamp's AsTime
// would give 1970, which looks like a real date.)
func toTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

func toProto(r domain.Rating) *freshnessv1.FreshnessScore {
	return &freshnessv1.FreshnessScore{
		Key:         &commonv1.StockKey{PharmacyId: r.PharmacyID, ProductId: r.ProductID},
		Score:       r.Confidence.Score(),
		ConfirmedAt: timestamppb.New(r.ConfirmedAt),
	}
}

// toStatus maps domain validation errors to INVALID_ARGUMENT; anything else is INTERNAL.
// The grpcx logging interceptor records the full error; callers only see a safe message.
func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrMissingPharmacyID),
		errors.Is(err, domain.ErrMissingProductID),
		errors.Is(err, domain.ErrMissingConfirmedAt):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
