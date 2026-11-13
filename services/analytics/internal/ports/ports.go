package ports

import (
	"context"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/domain"
)

type Repository interface {
	StoreSearchRecorded(context.Context, domain.SearchRecorded) error
	StoreStockUnavailable(context.Context, domain.StockUnavailable) error
	UnmetDemand(context.Context, domain.DemandQuery) ([]domain.DemandItem, error)
}

type PharmacyRegistry interface {
	GetPharmacy(context.Context, string) (string, error)
}

type Catalog interface {
	GetProduct(context.Context, string) (string, error)
}

type Clock func() time.Time
