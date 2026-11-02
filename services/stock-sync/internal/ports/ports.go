// Package ports defines the external services and storage the app layer needs.
package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/domain"
)

type Catalog interface {
	GetExistingProductIDs(ctx context.Context, productIDs []string) ([]string, error)
}

type Inventory interface {
	BulkSetStock(
		ctx context.Context,
		pharmacyID string,
		updates []domain.StockUpdate,
		updatedByUserID string,
	) (domain.BulkStockResult, error)
}

type Pricing interface {
	SetPrices(
		ctx context.Context,
		pharmacyID string,
		updates []domain.PriceUpdate,
	) error
}

type JobRepository interface {
	Save(ctx context.Context, job domain.SyncJob) error
	GetByID(ctx context.Context, jobID string) (domain.SyncJob, error)
	ListByPharmacy(
		ctx context.Context,
		pharmacyID string,
		pageSize int32,
		pageToken string,
	) ([]domain.SyncJob, string, error)
}
