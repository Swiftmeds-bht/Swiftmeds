package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	analyticsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/analytics/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/domain"
)

type Handler struct {
	analyticsv1.UnimplementedAnalyticsServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) GetUnmetDemand(ctx context.Context, req *analyticsv1.GetUnmetDemandRequest) (*analyticsv1.GetUnmetDemandResponse, error) {
	tr := req.GetRange()
	if tr == nil || tr.GetFrom() == nil || tr.GetTo() == nil {
		return nil, status.Error(codes.InvalidArgument, domain.ErrInvalidRange.Error())
	}
	items, err := h.svc.GetUnmetDemand(ctx, domain.DemandQuery{
		From: tr.GetFrom().AsTime(), To: tr.GetTo().AsTime(),
		Dzongkhag: req.GetDzongkhag(), Limit: int(req.GetLimit()),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*analyticsv1.UnmetDemandItem, 0, len(items))
	for _, item := range items {
		out = append(out, &analyticsv1.UnmetDemandItem{
			ProductId: item.ProductID, ProductName: item.ProductName,
			SearchCount: item.SearchCount, UnmetCount: item.UnmetCount,
		})
	}
	return &analyticsv1.GetUnmetDemandResponse{Items: out}, nil
}

func (h *Handler) GetPharmacyStats(context.Context, *analyticsv1.GetPharmacyStatsRequest) (*analyticsv1.GetPharmacyStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "pharmacy stats is not implemented")
}

func toStatus(err error) error {
	if errors.Is(err, domain.ErrInvalidRange) || errors.Is(err, domain.ErrMissingProduct) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Error(codes.Internal, "internal error")
}
