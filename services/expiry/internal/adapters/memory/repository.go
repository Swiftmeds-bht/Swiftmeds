// Package memory is an in-memory DeadlineRepository, used in tests.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/ports"
)

// Compile-time check that Repository satisfies the port.
var _ ports.DeadlineRepository = (*Repository)(nil)

type row struct {
	deadline  domain.Deadline
	completed bool
}

// Repository is safe for concurrent use.
type Repository struct {
	mu   sync.Mutex
	rows map[string]*row
}

func NewRepository() *Repository {
	return &Repository{rows: make(map[string]*row)}
}

func (r *Repository) Save(_ context.Context, d domain.Deadline) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.rows[d.HoldID]; !exists {
		r.rows[d.HoldID] = &row{deadline: d}
	}
	return nil
}

func (r *Repository) ListDue(_ context.Context, asOf time.Time, after *domain.Deadline, limit int) ([]domain.Deadline, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var due []domain.Deadline
	for _, rw := range r.rows {
		if rw.completed || !rw.deadline.Due(asOf) {
			continue
		}
		if after != nil && !comesAfter(rw.deadline, *after) {
			continue
		}
		due = append(due, rw.deadline)
	}
	sort.Slice(due, func(i, j int) bool { return comesBefore(due[i], due[j]) })
	if limit > 0 && len(due) > limit {
		due = due[:limit]
	}
	return due, nil
}

func (r *Repository) Complete(_ context.Context, holdID string, _ domain.Outcome, _ time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rw, ok := r.rows[holdID]
	if !ok || rw.completed {
		return false, nil
	}
	rw.completed = true
	return true, nil
}

// comesBefore is the listing order: by expiry time (in microseconds, the
// precision Postgres keeps), then by hold ID.
func comesBefore(a, b domain.Deadline) bool {
	if am, bm := a.ExpiresAt.UnixMicro(), b.ExpiresAt.UnixMicro(); am != bm {
		return am < bm
	}
	return a.HoldID < b.HoldID
}

func comesAfter(d, cursor domain.Deadline) bool {
	return comesBefore(cursor, d)
}
