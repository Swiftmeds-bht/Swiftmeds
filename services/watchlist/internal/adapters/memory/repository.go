// Package memory is an in-memory SubscriptionRepository for tests and local runs.
// It passes the same contract tests as the Postgres adapter.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/ports"
)

var _ ports.SubscriptionRepository = (*Repository)(nil)

type Repository struct {
	mu        sync.Mutex
	subs      map[string]domain.Subscription
	alerts    []domain.Alert
	delivered map[string]bool
}

func NewRepository() *Repository {
	return &Repository{subs: map[string]domain.Subscription{}, delivered: map[string]bool{}}
}

func (r *Repository) Create(_ context.Context, s domain.Subscription) (domain.Subscription, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.subs {
		if existing.UserID == s.UserID && existing.ProductID == s.ProductID && !existing.Notified() {
			return existing, false, nil
		}
	}
	r.subs[s.ID] = s
	return s, true, nil
}

func (r *Repository) Delete(_ context.Context, subscriptionID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.subs[subscriptionID]
	if !ok || s.UserID != userID {
		return domain.ErrNotFound
	}
	delete(r.subs, subscriptionID)
	kept := r.alerts[:0]
	for _, a := range r.alerts {
		if a.SubscriptionID != subscriptionID {
			kept = append(kept, a)
		}
	}
	r.alerts = kept
	return nil
}

func (r *Repository) ListByUser(_ context.Context, userID string) ([]domain.Subscription, error) {
	return r.filter(func(s domain.Subscription) bool { return s.UserID == userID }), nil
}

func (r *Repository) ActiveForProduct(_ context.Context, productID string) ([]domain.Subscription, error) {
	return r.filter(func(s domain.Subscription) bool { return s.ProductID == productID && !s.Notified() }), nil
}

func (r *Repository) Notify(_ context.Context, a domain.Alert) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.subs[a.SubscriptionID]
	if !ok || s.Notified() {
		return false, nil
	}
	s.NotifiedAt = a.OccurredAt
	r.subs[s.ID] = s
	r.alerts = append(r.alerts, a)
	return true, nil
}

func (r *Repository) PendingAlerts(_ context.Context, userID string) ([]domain.Alert, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.Alert{}
	for _, a := range r.alerts {
		if a.UserID == userID && !r.delivered[a.ID] {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].OccurredAt.Before(out[j].OccurredAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *Repository) MarkDelivered(_ context.Context, alertID string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delivered[alertID] = true
	return nil
}

// Alerts returns a copy of the saved alerts (for tests).
func (r *Repository) Alerts() []domain.Alert {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.Alert(nil), r.alerts...)
}

// filter returns matching subscriptions newest first; ties by ID so the order is stable.
func (r *Repository) filter(keep func(domain.Subscription) bool) []domain.Subscription {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.Subscription{}
	for _, s := range r.subs {
		if keep(s) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}
