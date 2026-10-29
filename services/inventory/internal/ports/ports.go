// Package ports defines the interfaces Inventory needs from adapters.
package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
)

type StockKey struct {
	PharmacyID string
	ProductID  string
}

type StockUpdate struct {
	ProductID      string
	QuantityOnHand int32
}

type StockChange struct {
	Stock             domain.Stock
	PreviousAvailable int32
}

type StockUpdateError struct {
	ProductID string
	Message   string
}

type StockRepository interface {
	Get(ctx context.Context, key StockKey) (stock domain.Stock, found bool, err error)
	GetMany(ctx context.Context, pharmacyIDs, productIDs []string, onlyAvailable bool) ([]domain.Stock, error)
	ListByPharmacy(ctx context.Context, pharmacyID string, pageSize int32, pageToken string) (stocks []domain.Stock, nextPageToken string, err error)

	Set(ctx context.Context, stock domain.Stock, updatedByUserID string) (previousAvailable int32, updated domain.Stock, err error)
	BulkSet(ctx context.Context, pharmacyID string, updates []StockUpdate, updatedByUserID string) (changes []StockChange, rowErrors []StockUpdateError, err error)

	// Reserve must atomically reduce available stock and be idempotent by holdID.
	Reserve(ctx context.Context, holdID string, key StockKey, quantity int32) (stock domain.Stock, reserved bool, err error)
	CommitReservation(ctx context.Context, holdID string) (domain.Stock, error)
	ReleaseReservation(ctx context.Context, holdID string) (domain.Stock, error)
}

type EventPublisher interface {
	PublishStockReserved(ctx context.Context, holdID, pharmacyID, productID string, quantity int32) error
	PublishStockUnavailable(ctx context.Context, holdID, pharmacyID, productID string, requestedQuantity, availableQuantity int32) error
	PublishStockIncreased(ctx context.Context, pharmacyID, productID string, previousAvailable, newAvailable int32) error
}
