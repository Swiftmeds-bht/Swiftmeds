// Package app holds Watchlist's use cases. It depends only on domain and ports.
package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/ports"
)

// Service implements the Watchlist use cases.
type Service struct {
	repo  ports.SubscriptionRepository
	geo   ports.PharmacyLocator
	now   func() time.Time
	newID func() string
}

// NewService wires the use cases. Pass time.Now and NewID in production, fixed ones in tests.
func NewService(repo ports.SubscriptionRepository, geo ports.PharmacyLocator, now func() time.Time, newID func() string) *Service {
	return &Service{repo: repo, geo: geo, now: now, newID: newID}
}

// Subscribe asks to be told when productID is in stock near a point. Subscribing
// again to the same product while waiting returns the existing subscription.
func (s *Service) Subscribe(ctx context.Context, userID, productID string, at domain.Point, radiusM int32) (domain.Subscription, error) {
	sub, err := domain.NewSubscription(s.newID(), userID, productID, at, radiusM, s.now())
	if err != nil {
		return domain.Subscription{}, err
	}
	got, _, err := s.repo.Create(ctx, sub)
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("save subscription: %w", err)
	}
	return got, nil
}

// Unsubscribe removes one of the user's subscriptions.
func (s *Service) Unsubscribe(ctx context.Context, subscriptionID, userID string) error {
	subscriptionID, userID = strings.TrimSpace(subscriptionID), strings.TrimSpace(userID)
	if subscriptionID == "" {
		return domain.ErrMissingSubscriptionID
	}
	if userID == "" {
		return domain.ErrMissingUserID
	}
	return s.repo.Delete(ctx, subscriptionID, userID)
}

// List returns the user's subscriptions, newest first, including notified ones.
func (s *Service) List(ctx context.Context, userID string) ([]domain.Subscription, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, domain.ErrMissingUserID
	}
	subs, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return subs, nil
}

// HandleStockIncreased records an alert for every waiting subscriber of the
// product whose radius includes the pharmacy, and returns those alerts.
//
// It is safe to run again for the same event: notified subscriptions are
// skipped, so a retry after a failure only alerts the ones not done yet.
func (s *Service) HandleStockIncreased(ctx context.Context, e domain.StockIncrease) ([]domain.Alert, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	if !e.InStock() {
		return nil, nil
	}
	subs, err := s.repo.ActiveForProduct(ctx, e.ProductID)
	if err != nil {
		return nil, fmt.Errorf("find subscriptions for %s: %w", e.ProductID, err)
	}

	var alerts []domain.Alert
	for _, sub := range subs {
		near, err := s.geo.WithinRadius(ctx, e.PharmacyID, sub.Location, sub.RadiusM)
		if err != nil {
			return alerts, fmt.Errorf("check distance for subscription %s: %w", sub.ID, err)
		}
		if !near {
			continue
		}
		a := domain.Alert{
			ID:                s.newID(),
			SubscriptionID:    sub.ID,
			UserID:            sub.UserID,
			ProductID:         sub.ProductID,
			PharmacyID:        e.PharmacyID,
			QuantityAvailable: e.NewAvailable,
			OccurredAt:        e.OccurredAt,
		}
		saved, err := s.repo.Notify(ctx, a)
		if err != nil {
			return alerts, fmt.Errorf("save alert for subscription %s: %w", sub.ID, err)
		}
		if saved {
			alerts = append(alerts, a)
		}
	}
	return alerts, nil
}

// NewID returns a random version-4 UUID.
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails on supported platforms
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
