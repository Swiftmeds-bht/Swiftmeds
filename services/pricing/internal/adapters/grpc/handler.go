// Package grpc is the inbound adapter: it translates PricingService RPCs into
// app calls and domain errors into gRPC status codes. No business rules here.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/domain"
)

// Handler implements pricingv1.PricingServiceServer.
type Handler struct {
	pricingv1.UnimplementedPricingServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) SetPrices(ctx context.Context, req *pricingv1.SetPricesRequest) (*pricingv1.SetPricesResponse, error) {
	updates := make([]app.Update, 0, len(req.GetUpdates()))
	for _, u := range req.GetUpdates() {
		updates = append(updates, app.Update{ProductID: u.GetProductId(), AmountChhertum: u.GetPriceChhertum()})
	}
	prices, err := h.svc.SetPrices(ctx, req.GetPharmacyId(), updates)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pricingv1.SetPricesResponse{Prices: toProto(prices)}, nil
}

func (h *Handler) BatchGetPrices(ctx context.Context, req *pricingv1.BatchGetPricesRequest) (*pricingv1.BatchGetPricesResponse, error) {
	keys := make([]domain.Key, 0, len(req.GetKeys()))
	for _, k := range req.GetKeys() {
		keys = append(keys, domain.Key{PharmacyID: k.GetPharmacyId(), ProductID: k.GetProductId()})
	}
	prices, err := h.svc.BatchGetPrices(ctx, keys)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pricingv1.BatchGetPricesResponse{Prices: toProto(prices)}, nil
}

func toProto(prices []domain.Price) []*pricingv1.Price {
	out := make([]*pricingv1.Price, 0, len(prices))
	for _, p := range prices {
		out = append(out, &pricingv1.Price{
			Key:           &commonv1.StockKey{PharmacyId: p.PharmacyID, ProductId: p.ProductID},
			PriceChhertum: p.AmountChhertum,
			UpdatedAt:     timestamppb.New(p.UpdatedAt),
		})
	}
	return out
}

// toStatus maps domain validation errors to INVALID_ARGUMENT; anything else is INTERNAL.
// The grpcx logging interceptor records the full error; callers only see a safe message.
func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrMissingPharmacyID),
		errors.Is(err, domain.ErrMissingProductID),
		errors.Is(err, domain.ErrNonPositivePrice):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
