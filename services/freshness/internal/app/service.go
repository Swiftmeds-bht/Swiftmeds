// Package app holds Freshness's use cases. It depends only on domain and ports.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/ports"
)

// Service implements the Freshness use cases.
type Service struct {
	repo       ports.RatingRepository
	thresholds domain.Thresholds
	now        func() time.Time
}

// NewService wires the use cases. Pass time.Now in production and a fixed clock in tests.
func NewService(repo ports.RatingRepository, thresholds domain.Thresholds, now func() time.Time) *Service {
	return &Service{repo: repo, thresholds: thresholds, now: now}
}

// RecordConfirmation stores when a stock row was confirmed and its confidence right now.
func (s *Service) RecordConfirmation(ctx context.Context, pharmacyID, productID string, confirmedAt time.Time) (domain.Rating, error) {
	c, err := domain.NewConfirmation(pharmacyID, productID, confirmedAt)
	if err != nil {
		return domain.Rating{}, err
	}
	rating := domain.Rating{Confirmation: c, Confidence: c.Confidence(s.thresholds, s.now())}

	saved, err := s.repo.Save(ctx, rating)
	if err != nil {
		return domain.Rating{}, fmt.Errorf("save confirmation: %w", err)
	}
	return saved, nil
}

// GetConfidence returns the stored rating of one stock row, or domain.ErrNotFound.
func (s *Service) GetConfidence(ctx context.Context, pharmacyID, productID string) (domain.Rating, error) {
	key, err := domain.NewKey(pharmacyID, productID)
	if err != nil {
		return domain.Rating{}, err
	}
	ratings, err := s.repo.GetMany(ctx, []domain.Key{key})
	if err != nil {
		return domain.Rating{}, fmt.Errorf("get rating: %w", err)
	}
	if len(ratings) == 0 {
		return domain.Rating{}, domain.ErrNotFound
	}
	return ratings[0], nil
}

// BatchGetConfidence returns the ratings that exist for keys. Keys that were
// never confirmed are skipped. One invalid key rejects the whole request.
func (s *Service) BatchGetConfidence(ctx context.Context, keys []domain.Key) ([]domain.Rating, error) {
	for i, k := range keys {
		if _, err := domain.NewKey(k.PharmacyID, k.ProductID); err != nil {
			return nil, fmt.Errorf("key %d: %w", i, err)
		}
	}
	ratings, err := s.repo.GetMany(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get ratings: %w", err)
	}
	return ratings, nil
}

// Decay recomputes every stored confidence as of asOf (the current time if asOf is zero)
// and returns how many rows changed. The result depends only on the stored
// confirmation times and asOf, so running it twice changes nothing the second time.
func (s *Service) Decay(ctx context.Context, asOf time.Time) (int, error) {
	if asOf.IsZero() {
		asOf = s.now()
	}
	all, err := s.repo.All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list ratings: %w", err)
	}

	var changed []domain.Rating
	for _, r := range all {
		if c := r.Confirmation.Confidence(s.thresholds, asOf); c != r.Confidence {
			r.Confidence = c
			changed = append(changed, r)
		}
	}
	if len(changed) == 0 {
		return 0, nil
	}
	if err := s.repo.UpdateConfidence(ctx, changed); err != nil {
		return 0, fmt.Errorf("update confidence: %w", err)
	}
	return len(changed), nil
}
