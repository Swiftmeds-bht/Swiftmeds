package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/ports"
)

var _ ports.PharmacyRepository = (*Repository)(nil)

type Repository struct {
	mu         sync.RWMutex
	pharmacies map[string]domain.Pharmacy
}

func NewRepository() *Repository {
	return &Repository{pharmacies: make(map[string]domain.Pharmacy)}
}

func (r *Repository) Create(_ context.Context, p domain.Pharmacy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pharmacies[p.ID] = p
	return nil
}

func (r *Repository) Get(_ context.Context, id string) (domain.Pharmacy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.pharmacies[id]
	if !ok {
		return domain.Pharmacy{}, domain.ErrNotFound
	}
	return p, nil
}

func (r *Repository) List(_ context.Context, verifiedOnly bool) ([]domain.Pharmacy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Pharmacy, 0, len(r.pharmacies))
	for _, p := range r.pharmacies {
		if !verifiedOnly || p.Status == domain.StatusVerified {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *Repository) SetStatus(_ context.Context, id string, status domain.Status, reason string, now time.Time) (domain.Pharmacy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pharmacies[id]
	if !ok {
		return domain.Pharmacy{}, domain.ErrNotFound
	}
	next, err := p.ApplyStatus(status, reason, now)
	if err != nil {
		return domain.Pharmacy{}, err
	}
	r.pharmacies[id] = next
	return next, nil
}
