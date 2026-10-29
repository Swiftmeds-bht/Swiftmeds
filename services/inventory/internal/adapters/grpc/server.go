package grpc

import (
	"context"
	"errors"

	"github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server adapts the Inventory application service to gRPC.
type Server struct {
	inventoryv1.UnimplementedInventoryServiceServer
	service *app.Service
}

func NewServer(service *app.Service) (*Server, error) {
	if service == nil {
		return nil, errors.New("inventory application service is nil")
	}
	return &Server{service: service}, nil
}

func (s *Server) GetStock(
	ctx context.Context,
	req *inventoryv1.GetStockRequest,
) (*inventoryv1.GetStockResponse, error) {
	stock, err := s.service.GetStock(ctx, req.GetPharmacyId(), req.GetProductId())
	if err != nil {
		return nil, toStatusError(err)
	}
	return &inventoryv1.GetStockResponse{Stock: toStockLevel(stock)}, nil
}

func (s *Server) BatchGetStock(
	ctx context.Context,
	req *inventoryv1.BatchGetStockRequest,
) (*inventoryv1.BatchGetStockResponse, error) {
	stocks, err := s.service.BatchGetStock(
		ctx,
		req.GetPharmacyIds(),
		req.GetProductIds(),
		req.GetOnlyAvailable(),
	)
	if err != nil {
		return nil, toStatusError(err)
	}

	response := &inventoryv1.BatchGetStockResponse{
		Stock: make([]*inventoryv1.StockLevel, 0, len(stocks)),
	}
	for _, stock := range stocks {
		response.Stock = append(response.Stock, toStockLevel(stock))
	}
	return response, nil
}

func (s *Server) ListPharmacyStock(
	ctx context.Context,
	req *inventoryv1.ListPharmacyStockRequest,
) (*inventoryv1.ListPharmacyStockResponse, error) {
	page := req.GetPage()
	stocks, nextPageToken, err := s.service.ListPharmacyStock(
		ctx,
		req.GetPharmacyId(),
		page.GetPageSize(),
		page.GetPageToken(),
	)
	if err != nil {
		return nil, toStatusError(err)
	}

	response := &inventoryv1.ListPharmacyStockResponse{
		Stock: make([]*inventoryv1.StockLevel, 0, len(stocks)),
		Page:  &commonv1.PageResponse{NextPageToken: nextPageToken},
	}
	for _, stock := range stocks {
		response.Stock = append(response.Stock, toStockLevel(stock))
	}
	return response, nil
}

func (s *Server) SetStock(
	ctx context.Context,
	req *inventoryv1.SetStockRequest,
) (*inventoryv1.SetStockResponse, error) {
	stock, err := s.service.SetStock(
		ctx,
		req.GetPharmacyId(),
		req.GetProductId(),
		req.GetQuantityOnHand(),
		req.GetUpdatedByUserId(),
	)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &inventoryv1.SetStockResponse{Stock: toStockLevel(stock)}, nil
}

func (s *Server) BulkSetStock(
	ctx context.Context,
	req *inventoryv1.BulkSetStockRequest,
) (*inventoryv1.BulkSetStockResponse, error) {
	updates := make([]ports.StockUpdate, 0, len(req.GetUpdates()))
	for _, update := range req.GetUpdates() {
		if update == nil {
			updates = append(updates, ports.StockUpdate{})
			continue
		}
		updates = append(updates, ports.StockUpdate{
			ProductID:      update.GetProductId(),
			QuantityOnHand: update.GetQuantityOnHand(),
		})
	}

	changes, rowErrors, err := s.service.BulkSetStock(
		ctx,
		req.GetPharmacyId(),
		updates,
		req.GetUpdatedByUserId(),
	)
	if err != nil {
		return nil, toStatusError(err)
	}

	response := &inventoryv1.BulkSetStockResponse{
		UpdatedCount: int32(len(changes)),
		Errors:       make([]*inventoryv1.StockUpdateError, 0, len(rowErrors)),
	}
	for _, rowError := range rowErrors {
		response.Errors = append(response.Errors, &inventoryv1.StockUpdateError{
			ProductId: rowError.ProductID,
			Message:   rowError.Message,
		})
	}
	return response, nil
}

func (s *Server) CommitReservation(
	ctx context.Context,
	req *inventoryv1.CommitReservationRequest,
) (*inventoryv1.CommitReservationResponse, error) {
	stock, err := s.service.CommitReservation(ctx, req.GetHoldId())
	if err != nil {
		return nil, toStatusError(err)
	}
	return &inventoryv1.CommitReservationResponse{Stock: toStockLevel(stock)}, nil
}

func (s *Server) ReleaseReservation(
	ctx context.Context,
	req *inventoryv1.ReleaseReservationRequest,
) (*inventoryv1.ReleaseReservationResponse, error) {
	stock, err := s.service.ReleaseReservation(ctx, req.GetHoldId())
	if err != nil {
		return nil, toStatusError(err)
	}
	return &inventoryv1.ReleaseReservationResponse{Stock: toStockLevel(stock)}, nil
}

func toStockLevel(stock domain.Stock) *inventoryv1.StockLevel {
	return &inventoryv1.StockLevel{
		PharmacyId:        stock.PharmacyID,
		ProductId:         stock.ProductID,
		QuantityOnHand:    stock.QuantityOnHand,
		QuantityReserved:  stock.QuantityReserved,
		QuantityAvailable: stock.QuantityAvailable(),
		ConfirmedAt:       timestamppb.New(stock.ConfirmedAt),
	}
}

func toStatusError(err error) error {
	switch {
	case errors.Is(err, app.ErrStockNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidStockKey),
		errors.Is(err, domain.ErrNegativeStock),
		errors.Is(err, domain.ErrInvalidReservationQuantity):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrInsufficientAvailableStock):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, domain.ErrOnHandBelowReservedQuantity),
		errors.Is(err, domain.ErrInsufficientReservedStock):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, "inventory operation failed")
	}
}

var _ inventoryv1.InventoryServiceServer = (*Server)(nil)
