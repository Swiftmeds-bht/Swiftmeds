package memory

import (
	"context"
	"sync"

	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/ports"
)

var _ ports.ProfileRepository = (*Repository)(nil)

type Repository struct {
	mu       sync.RWMutex
	profiles map[string]domain.Profile
}

func NewRepository() *Repository { return &Repository{profiles: make(map[string]domain.Profile)} }
func (r *Repository) Create(_ context.Context, p domain.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.profiles[p.UserID]; ok {
		return domain.ErrProfileExists
	}
	r.profiles[p.UserID] = p
	return nil
}
func (r *Repository) Get(_ context.Context, id string) (domain.Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[id]
	if !ok {
		return domain.Profile{}, domain.ErrProfileNotFound
	}
	return p, nil
}
func (r *Repository) Update(_ context.Context, p domain.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.profiles[p.UserID]; !ok {
		return domain.ErrProfileNotFound
	}
	r.profiles[p.UserID] = p
	return nil
}
