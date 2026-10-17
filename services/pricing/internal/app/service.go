// Package app holds Pricing's use cases. It depends only on domain and ports.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/ports"
)

// Update is one requested price change.
type Update struct {
	ProductID      string
	AmountChhertum int64
}

// Service implements the Pricing use cases.
type Service struct {
	repo ports.PriceRepository
	now  func() time.Time
}

// NewService wires the use cases. Pass time.Now in production and a fixed clock in tests.
func NewService(repo ports.PriceRepository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

// SetPrices validates every update first and saves none if any is invalid.
func (s *Service) SetPrices(ctx context.Context, pharmacyID string, updates []Update) ([]domain.Price, error) {
	now := s.now()
	prices := make([]domain.Price, 0, len(updates))
	for i, u := range updates {
		p, err := domain.NewPrice(pharmacyID, u.ProductID, u.AmountChhertum, now)
		if err != nil {
			return nil, fmt.Errorf("update %d: %w", i, err)
		}
		prices = append(prices, p)
	}
	if err := s.repo.SaveAll(ctx, prices); err != nil {
		return nil, fmt.Errorf("save prices: %w", err)
	}
	return prices, nil
}

// BatchGetPrices returns the prices that exist for keys.
func (s *Service) BatchGetPrices(ctx context.Context, keys []domain.Key) ([]domain.Price, error) {
	prices, err := s.repo.GetMany(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get prices: %w", err)
	}
	return prices, nil
}
