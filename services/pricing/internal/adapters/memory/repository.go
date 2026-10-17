// Package memory is an in-memory PriceRepository.
// It keeps the template runnable without a database; the postgres adapter replaces it.
package memory

import (
	"context"
	"sync"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/ports"
)

// Compile-time check that Repository satisfies the port.
var _ ports.PriceRepository = (*Repository)(nil)

// Repository is safe for concurrent use.
type Repository struct {
	mu     sync.RWMutex
	prices map[domain.Key]domain.Price
}

func NewRepository() *Repository {
	return &Repository{prices: make(map[domain.Key]domain.Price)}
}

func (r *Repository) SaveAll(_ context.Context, prices []domain.Price) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range prices {
		r.prices[p.Key] = p
	}
	return nil
}

func (r *Repository) GetMany(_ context.Context, keys []domain.Key) ([]domain.Price, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Price, 0, len(keys))
	for _, k := range keys {
		if p, ok := r.prices[k]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}
