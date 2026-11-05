// Package app holds Geo's use cases. It depends only on domain and ports.
package app

import (
	"context"
	"fmt"

	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/ports"
)

type Service struct {
	repo ports.LocationRepository
}

func NewService(repo ports.LocationRepository) *Service {
	return &Service{repo: repo}
}

// FindNearby returns verified pharmacies within radius of center, nearest first.
// The repository narrows candidates with a bounding box; the domain does the
// exact distance check, ordering and limit.
func (s *Service) FindNearby(ctx context.Context, center domain.Point, radiusMetres, limit int32) ([]domain.Nearby, error) {
	q, err := domain.NewQuery(center, radiusMetres, limit)
	if err != nil {
		return nil, err
	}
	candidates, err := s.repo.WithinBox(ctx, domain.BoundingBox(q.Center, float64(q.RadiusMetres)))
	if err != nil {
		return nil, fmt.Errorf("find candidates: %w", err)
	}
	return q.Find(candidates), nil
}

// UpsertLocation records where a pharmacy is. Pharmacy Registry calls it when a
// pharmacy is verified or moves.
func (s *Service) UpsertLocation(ctx context.Context, loc domain.Location) error {
	if err := loc.Validate(); err != nil {
		return err
	}
	if err := s.repo.Upsert(ctx, loc); err != nil {
		return fmt.Errorf("upsert location: %w", err)
	}
	return nil
}

// RemoveLocation stops a pharmacy appearing in results (e.g. when suspended).
// Removing an unknown pharmacy succeeds, so callers can retry safely.
func (s *Service) RemoveLocation(ctx context.Context, pharmacyID string) error {
	if pharmacyID == "" {
		return domain.ErrMissingPharmacyID
	}
	if err := s.repo.Remove(ctx, pharmacyID); err != nil {
		return fmt.Errorf("remove location: %w", err)
	}
	return nil
}
