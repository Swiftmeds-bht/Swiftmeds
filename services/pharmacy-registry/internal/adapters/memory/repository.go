package memory

import (
	"context"
	"encoding/base64"
	"sort"
	"strings"
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
	for _, other := range r.pharmacies {
		if other.LicenseNumber == p.LicenseNumber {
			return domain.ErrLicenseTaken
		}
	}
	r.pharmacies[p.ID] = p
	return nil
}

func (r *Repository) FindByOwner(_ context.Context, ownerID string) (domain.Pharmacy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var found *domain.Pharmacy
	for _, p := range r.pharmacies {
		if p.OwnerID != ownerID {
			continue
		}
		if found == nil || p.CreatedAt.Before(found.CreatedAt) || (p.CreatedAt.Equal(found.CreatedAt) && p.ID < found.ID) {
			p := p
			found = &p
		}
	}
	if found == nil {
		return domain.Pharmacy{}, domain.ErrNotFound
	}
	return *found, nil
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

func (r *Repository) GetMany(_ context.Context, ids []string) ([]domain.Pharmacy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Pharmacy, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if p, ok := r.pharmacies[id]; ok {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *Repository) ListPage(_ context.Context, status domain.Status, pageSize int, pageToken string) ([]domain.Pharmacy, string, error) {
	createdAt, id, err := decodeCursor(pageToken)
	if err != nil {
		return nil, "", err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]domain.Pharmacy, 0, len(r.pharmacies))
	for _, p := range r.pharmacies {
		if status != "" && p.Status != status {
			continue
		}
		if pageToken != "" && !(p.CreatedAt.After(createdAt) || (p.CreatedAt.Equal(createdAt) && p.ID > id)) {
			continue
		}
		all = append(all, p)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID < all[j].ID
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})
	if len(all) <= pageSize {
		return all, "", nil
	}
	all = all[:pageSize]
	last := all[len(all)-1]
	return all, encodeCursor(last.CreatedAt, last.ID), nil
}

func encodeCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}
func decodeCursor(token string) (time.Time, string, error) {
	if token == "" {
		return time.Time{}, "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, "", domain.ErrInvalidPageToken
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", domain.ErrInvalidPageToken
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", domain.ErrInvalidPageToken
	}
	return at, parts[1], nil
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

func (r *Repository) Update(_ context.Context, id, ownerID string, changes domain.Pharmacy) (domain.Pharmacy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pharmacies[id]
	if !ok {
		return domain.Pharmacy{}, domain.ErrNotFound
	}
	if p.OwnerID != ownerID {
		return domain.Pharmacy{}, domain.ErrOwnerMismatch
	}
	p.Name, p.Phone, p.Address, p.Dzongkhag = changes.Name, changes.Phone, changes.Address, changes.Dzongkhag
	p.Location, p.Hours = changes.Location, append([]domain.OpeningHours(nil), changes.Hours...)
	if p.Status == domain.StatusRejected {
		p.Status, p.StatusReason = domain.StatusPending, ""
	}
	r.pharmacies[id] = p
	return p, nil
}
