// Package memory is an in-memory TrustRepository for tests and local runs.
// It passes the same contract tests as the Postgres adapter.
package memory

import (
	"context"
	"sync"

	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/ports"
)

var _ ports.TrustRepository = (*Repository)(nil)

type Repository struct {
	mu      sync.Mutex
	records map[string]domain.Record
	applied map[string]bool // feedback IDs already counted
}

func NewRepository() *Repository {
	return &Repository{records: map[string]domain.Record{}, applied: map[string]bool{}}
}

func (r *Repository) ApplyFeedback(_ context.Context, f domain.Feedback, update func(domain.Record) domain.Record) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.applied[f.ID] {
		return false, nil
	}
	rec, ok := r.records[f.PharmacyID]
	if !ok {
		rec = domain.Record{PharmacyID: f.PharmacyID}
	}
	rec = update(rec)
	rec.PharmacyID = f.PharmacyID
	r.records[f.PharmacyID] = rec
	r.applied[f.ID] = true
	return true, nil
}

func (r *Repository) Records(_ context.Context, pharmacyIDs []string) (map[string]domain.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]domain.Record, len(pharmacyIDs))
	for _, id := range pharmacyIDs {
		if rec, ok := r.records[id]; ok {
			out[id] = rec
		}
	}
	return out, nil
}
