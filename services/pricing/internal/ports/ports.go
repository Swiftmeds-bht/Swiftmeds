// Package ports defines the interfaces the app layer needs from the outside world.
// Adapters (postgres, memory, ...) implement them; tests use fakes.
package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/domain"
)

// PriceRepository stores prices.
type PriceRepository interface {
	// SaveAll inserts or replaces every price, all or nothing.
	SaveAll(ctx context.Context, prices []domain.Price) error
	// GetMany returns the prices that exist for keys; missing keys are skipped.
	GetMany(ctx context.Context, keys []domain.Key) ([]domain.Price, error)
}
