package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/ports"
)

const DefaultLimit = 10

type Service struct {
	repo     ports.Repository
	registry ports.PharmacyRegistry
	catalog  ports.Catalog
	now      ports.Clock
}

func NewService(repo ports.Repository, registry ports.PharmacyRegistry, catalog ports.Catalog, now ports.Clock) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, registry: registry, catalog: catalog, now: now}
}

func (s *Service) RecordSearch(ctx context.Context, e domain.SearchRecorded) error {
	if e.ProductID == "" {
		return domain.ErrMissingProduct
	}
	return s.repo.StoreSearchRecorded(ctx, e)
}

func (s *Service) RecordStockUnavailable(ctx context.Context, e domain.StockUnavailable) error {
	if e.ProductID == "" {
		return domain.ErrMissingProduct
	}
	if s.registry == nil || s.catalog == nil {
		return fmt.Errorf("lookup clients are unavailable")
	}
	dzongkhag, err := s.registry.GetPharmacy(ctx, e.PharmacyID)
	if err != nil {
		return fmt.Errorf("get pharmacy: %w", err)
	}
	productName, err := s.catalog.GetProduct(ctx, e.ProductID)
	if err != nil {
		return fmt.Errorf("get product: %w", err)
	}
	e.Dzongkhag = strings.TrimSpace(dzongkhag)
	e.ProductName = productName
	if e.OccurredAt.IsZero() {
		e.OccurredAt = s.now()
	}
	return s.repo.StoreStockUnavailable(ctx, e)
}

func (s *Service) GetUnmetDemand(ctx context.Context, q domain.DemandQuery) ([]domain.DemandItem, error) {
	if q.Limit <= 0 {
		q.Limit = DefaultLimit
	}
	q.Dzongkhag = strings.TrimSpace(q.Dzongkhag)
	if err := q.Validate(); err != nil {
		return nil, err
	}
	return s.repo.UnmetDemand(ctx, q)
}
