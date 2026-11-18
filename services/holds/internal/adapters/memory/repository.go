// Package memory is an in-memory HoldRepository, used in tests and for
// running the service without a database.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

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

func (r *Repository) Create(_ context.Context, h domain.Hold, maxOpen int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.holds[h.ID]; exists {
		return domain.ErrAlreadyExists
	}
	if maxOpen > 0 && r.openHolds(h.UserID) >= maxOpen {
		return domain.ErrLimitReached
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
	if h.Status == domain.Active && r.codeInUse(h) {
		return domain.ErrPickupCodeTaken
	}
	// Only RecordWrongPickupCode changes the count; keep the stored one.
	h.PickupAttempts = stored.PickupAttempts
	r.holds[h.ID] = h
	return nil
}

func (r *Repository) RecordWrongPickupCode(_ context.Context, id string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.holds[id]
	if !ok {
		return 0, domain.ErrNotFound
	}
	if h.Status != domain.Active {
		return 0, domain.ErrConflict
	}
	h.PickupAttempts++
	r.holds[id] = h
	return h.PickupAttempts, nil
}

func (r *Repository) ListStalePending(_ context.Context, createdBefore time.Time, limit int) ([]domain.Hold, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domain.Hold
	for _, h := range r.holds {
		if h.Status == domain.Pending && h.CreatedAt.UnixMicro() < createdBefore.UnixMicro() {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool { return oldestFirst(out[i].CreatedAt, out[i].ID, out[j].CreatedAt, out[j].ID) })
	return firstN(out, limit), nil
}

func (r *Repository) ListReleasePending(_ context.Context, limit int) ([]domain.Hold, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domain.Hold
	for _, h := range r.holds {
		if h.StockReleasePending {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return oldestFirst(out[i].StatusChangedAt, out[i].ID, out[j].StatusChangedAt, out[j].ID)
	})
	return firstN(out, limit), nil
}

// oldestFirst orders by time, then by ID, in microseconds like Postgres.
func oldestFirst(at time.Time, aID string, bt time.Time, bID string) bool {
	if am, bm := at.UnixMicro(), bt.UnixMicro(); am != bm {
		return am < bm
	}
	return aID < bID
}

func firstN(holds []domain.Hold, limit int) []domain.Hold {
	if limit > 0 && len(holds) > limit {
		return holds[:limit]
	}
	return holds
}

func (r *Repository) List(_ context.Context, f domain.ListFilter) ([]domain.Hold, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domain.Hold
	for _, h := range r.holds {
		switch {
		case f.UserID != "" && h.UserID != f.UserID:
			continue
		case f.PharmacyID != "" && h.PharmacyID != f.PharmacyID:
			continue
		case f.Status != "" && h.Status != f.Status:
			continue
		case f.After != nil && !comesAfter(h, *f.After):
			continue
		}
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return comesFirst(out[i], out[j]) })
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// openHolds counts the patient's PENDING and ACTIVE holds. The caller holds the lock.
func (r *Repository) openHolds(userID string) int {
	n := 0
	for _, h := range r.holds {
		if h.UserID == userID && h.Status.IsOpen() {
			n++
		}
	}
	return n
}

// codeInUse reports whether another ACTIVE hold at the same pharmacy has h's
// pickup code. The caller holds the lock.
func (r *Repository) codeInUse(h domain.Hold) bool {
	for _, other := range r.holds {
		if other.ID != h.ID && other.Status == domain.Active &&
			other.PharmacyID == h.PharmacyID && other.PickupCode == h.PickupCode {
			return true
		}
	}
	return false
}

// comesFirst orders holds newest first, then by ID, newest ID first. Times are
// compared in microseconds, the precision Postgres keeps.
func comesFirst(a, b domain.Hold) bool {
	if am, bm := a.CreatedAt.UnixMicro(), b.CreatedAt.UnixMicro(); am != bm {
		return am > bm
	}
	return a.ID > b.ID
}

// comesAfter reports whether h is listed after the cursor.
func comesAfter(h domain.Hold, c domain.Cursor) bool {
	if hm, cm := h.CreatedAt.UnixMicro(), c.CreatedAt.UnixMicro(); hm != cm {
		return hm < cm
	}
	return h.ID < c.ID
}
