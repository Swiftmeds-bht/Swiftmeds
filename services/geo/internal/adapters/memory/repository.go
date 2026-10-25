// Package memory is an in-memory LocationRepository, used in tests.
package memory

import (
	"context"
	"sync"

	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/ports"
)

var _ ports.LocationRepository = (*Repository)(nil)

type Repository struct {
	mu   sync.RWMutex
	locs map[string]domain.Point
}

func NewRepository() *Repository {
	return &Repository{locs: make(map[string]domain.Point)}
}

func (r *Repository) Upsert(_ context.Context, loc domain.Location) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.locs[loc.PharmacyID] = loc.Point
	return nil
}

func (r *Repository) Remove(_ context.Context, pharmacyID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.locs, pharmacyID)
	return nil
}

func (r *Repository) WithinBox(_ context.Context, b domain.Box) ([]domain.Location, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domain.Location
	for id, p := range r.locs {
		if p.Lat >= b.MinLat && p.Lat <= b.MaxLat && p.Lng >= b.MinLng && p.Lng <= b.MaxLng {
			out = append(out, domain.Location{PharmacyID: id, Point: p})
		}
	}
	return out, nil
}
