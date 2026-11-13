// Package grpc is the inbound adapter: it translates TrustService RPCs into
// app calls and domain errors into gRPC status codes. No business rules here.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	trustv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/trust/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
)

// Handler implements trustv1.TrustServiceServer.
type Handler struct {
	trustv1.UnimplementedTrustServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GetTrust(ctx context.Context, req *trustv1.GetTrustRequest) (*trustv1.GetTrustResponse, error) {
	t, err := h.svc.GetTrust(ctx, req.GetPharmacyId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &trustv1.GetTrustResponse{Score: toProto(t)}, nil
}

func (h *Handler) BatchGetTrust(ctx context.Context, req *trustv1.BatchGetTrustRequest) (*trustv1.BatchGetTrustResponse, error) {
	ts, err := h.svc.BatchGetTrust(ctx, req.GetPharmacyIds())
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*trustv1.TrustScore, len(ts))
	for i, t := range ts {
		out[i] = toProto(t)
	}
	return &trustv1.BatchGetTrustResponse{Scores: out}, nil
}

func toProto(t domain.Trust) *trustv1.TrustScore {
	p := &trustv1.TrustScore{
		PharmacyId:    t.PharmacyID,
		Score:         t.Score,
		FeedbackCount: t.FeedbackCount,
		InStockCount:  t.InStockCount,
	}
	if !t.UpdatedAt.IsZero() {
		p.UpdatedAt = timestamppb.New(t.UpdatedAt)
	}
	return p
}

// toStatus maps request validation errors to INVALID_ARGUMENT; anything else
// is INTERNAL. The grpcx logging interceptor records the full error; callers
// only see a safe message.
func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrMissingPharmacyID), errors.Is(err, domain.ErrTooManyPharmacies):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
