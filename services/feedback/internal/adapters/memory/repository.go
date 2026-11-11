// Package memory is an in-memory FeedbackRepository for tests. It behaves like
// the Postgres one and passes the same contract tests.
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/ports"
)

var _ ports.FeedbackRepository = (*Repository)(nil)

type Repository struct {
	mu     sync.Mutex
	byHold map[string]domain.Feedback
}

func NewRepository() *Repository {
	return &Repository{byHold: make(map[string]domain.Feedback)}
}

// Save stores f unless its hold already has feedback. The lock is held while
// beforeCommit runs, so two saves for one hold cannot both pass.
func (r *Repository) Save(ctx context.Context, f domain.Feedback, beforeCommit func(context.Context) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, taken := r.byHold[f.HoldID]; taken {
		return domain.ErrAlreadySubmitted
	}
	if beforeCommit != nil {
		if err := beforeCommit(ctx); err != nil {
			return err
		}
	}
	r.byHold[f.HoldID] = f
	return nil
}

func (r *Repository) ListByPharmacy(_ context.Context, filter domain.ListFilter) ([]domain.Feedback, error) {
	r.mu.Lock()
	var out []domain.Feedback
	for _, f := range r.byHold {
		if f.PharmacyID != filter.PharmacyID {
			continue
		}
		if filter.After != nil && !before(f, *filter.After) {
			continue
		}
		out = append(out, f)
	}
	r.mu.Unlock()

	// Newest first, then by ID, the same order as the Postgres query.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

// before reports whether f sorts after the cursor in the newest-first order,
// that is, whether it belongs on a later page.
func before(f domain.Feedback, c domain.Cursor) bool {
	if !f.CreatedAt.Equal(c.CreatedAt) {
		return f.CreatedAt.Before(c.CreatedAt)
	}
	return f.ID < c.ID
}
