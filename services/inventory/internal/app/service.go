package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
)

var ErrStockNotFound = errors.New("inventory stock not found")

// Service coordinates Inventory use cases.
type Service struct {
	repo      ports.StockRepository
	publisher ports.EventPublisher
	now       func() time.Time
}

// NewService wires the Inventory use cases.
func NewService(
	repo ports.StockRepository,
	publisher ports.EventPublisher,
	now func() time.Time,
) (*Service, error) {
	if repo == nil {
		return nil, errors.New("inventory repository is nil")
	}
	if publisher == nil {
		return nil, errors.New("inventory event publisher is nil")
	}
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, publisher: publisher, now: now}, nil
}

func (s *Service) GetStock(
	ctx context.Context,
	pharmacyID, productID string,
) (domain.Stock, error) {
	stock, found, err := s.repo.Get(ctx, ports.StockKey{
		PharmacyID: pharmacyID,
		ProductID:  productID,
	})
	if err != nil {
		return domain.Stock{}, fmt.Errorf("get inventory stock: %w", err)
	}
	if !found {
		return domain.Stock{}, fmt.Errorf(
			"%w: pharmacy %q, product %q",
			ErrStockNotFound,
			pharmacyID,
			productID,
		)
	}
	return stock, nil
}

func (s *Service) BatchGetStock(
	ctx context.Context,
	pharmacyIDs, productIDs []string,
	onlyAvailable bool,
) ([]domain.Stock, error) {
	stocks, err := s.repo.GetMany(ctx, pharmacyIDs, productIDs, onlyAvailable)
	if err != nil {
		return nil, fmt.Errorf("batch get inventory stock: %w", err)
	}
	return stocks, nil
}

func (s *Service) ListPharmacyStock(
	ctx context.Context,
	pharmacyID string,
	pageSize int32,
	pageToken string,
) ([]domain.Stock, string, error) {
	stocks, nextPageToken, err := s.repo.ListByPharmacy(
		ctx,
		pharmacyID,
		pageSize,
		pageToken,
	)
	if err != nil {
		return nil, "", fmt.Errorf("list pharmacy stock: %w", err)
	}
	return stocks, nextPageToken, nil
}

func (s *Service) SetStock(
	ctx context.Context,
	pharmacyID, productID string,
	quantityOnHand int32,
	updatedByUserID string,
) (domain.Stock, error) {
	requested, err := domain.NewStock(
		pharmacyID,
		productID,
		quantityOnHand,
		0,
		s.now().UTC(),
	)
	if err != nil {
		return domain.Stock{}, err
	}

	previousAvailable, updated, err := s.repo.Set(ctx, requested, updatedByUserID)
	if err != nil {
		return domain.Stock{}, fmt.Errorf("set inventory stock: %w", err)
	}

	newAvailable := updated.QuantityAvailable()
	if newAvailable > previousAvailable {
		if err := s.publisher.PublishStockIncreased(
			ctx,
			pharmacyID,
			productID,
			previousAvailable,
			newAvailable,
		); err != nil {
			return updated, fmt.Errorf("publish stock increased event: %w", err)
		}
	}
	return updated, nil
}

func (s *Service) BulkSetStock(
	ctx context.Context,
	pharmacyID string,
	updates []ports.StockUpdate,
	updatedByUserID string,
) ([]ports.StockChange, []ports.StockUpdateError, error) {
	changes, rowErrors, err := s.repo.BulkSet(
		ctx,
		pharmacyID,
		updates,
		updatedByUserID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("bulk set inventory stock: %w", err)
	}

	for _, change := range changes {
		newAvailable := change.Stock.QuantityAvailable()
		if newAvailable <= change.PreviousAvailable {
			continue
		}
		if err := s.publisher.PublishStockIncreased(
			ctx,
			pharmacyID,
			change.Stock.ProductID,
			change.PreviousAvailable,
			newAvailable,
		); err != nil {
			return changes, rowErrors, fmt.Errorf("publish stock increased event: %w", err)
		}
	}
	return changes, rowErrors, nil
}

func (s *Service) Reserve(
	ctx context.Context,
	holdID string,
	key ports.StockKey,
	quantity int32,
) (domain.Stock, bool, error) {
	stock, reserved, err := s.repo.Reserve(ctx, holdID, key, quantity)
	if err != nil {
		return domain.Stock{}, false, fmt.Errorf("reserve inventory: %w", err)
	}

	if reserved {
		err = s.publisher.PublishStockReserved(
			ctx,
			holdID,
			key.PharmacyID,
			key.ProductID,
			quantity,
		)
	} else {
		err = s.publisher.PublishStockUnavailable(
			ctx,
			holdID,
			key.PharmacyID,
			key.ProductID,
			quantity,
			stock.QuantityAvailable(),
		)
	}
	if err != nil {
		return stock, reserved, fmt.Errorf("publish reservation event: %w", err)
	}
	return stock, reserved, nil
}

func (s *Service) CommitReservation(
	ctx context.Context,
	holdID string,
) (domain.Stock, error) {
	stock, err := s.repo.CommitReservation(ctx, holdID)
	if err != nil {
		return domain.Stock{}, fmt.Errorf("commit inventory reservation: %w", err)
	}
	return stock, nil
}

func (s *Service) ReleaseReservation(
	ctx context.Context,
	holdID string,
) (domain.Stock, error) {
	stock, err := s.repo.ReleaseReservation(ctx, holdID)
	if err != nil {
		return domain.Stock{}, fmt.Errorf("release inventory reservation: %w", err)
	}
	return stock, nil
}
