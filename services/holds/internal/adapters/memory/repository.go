// Package memory is an in-memory HoldRepository, used in tests and for
// running the service without a database.
package memory

import (
	"context"
	"sync"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

// Compile-time check that Repository satisfies the port.
var _ ports.HoldRepository = (*Repository)(nil)

// Repository is safe for concurrent use.
type Repository struct {
	mu    sync.RWMutex
	holds map[string]domain.Hold
}

func NewRepository() *Repository {
	return &Repository{holds: make(map[string]domain.Hold)}
}

func (r *Repository) Create(_ context.Context, h domain.Hold) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.holds[h.ID]; exists {
		return domain.ErrAlreadyExists
	}
	r.holds[h.ID] = h
	return nil
}

func (r *Repository) Get(_ context.Context, id string) (domain.Hold, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.holds[id]
	if !ok {
		return domain.Hold{}, domain.ErrNotFound
	}
	return h, nil
}

func (r *Repository) Update(_ context.Context, h domain.Hold, from domain.Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.holds[h.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if stored.Status != from {
		return domain.ErrConflict
	}
	r.holds[h.ID] = h
	return nil
}
