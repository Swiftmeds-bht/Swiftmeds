// Package ports defines the interfaces the app layer needs from the outside world.
// Adapters (postgres, ...) implement them; tests use fakes.
package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
)

// RatingRepository stores each stock row's last confirmation and its confidence.
type RatingRepository interface {
	// Save inserts or updates a rating. A confirmation older than the stored one
	// never replaces it. It returns the rating as stored.
	Save(ctx context.Context, r domain.Rating) (domain.Rating, error)
	// GetMany returns the ratings that exist for keys; missing keys are skipped.
	GetMany(ctx context.Context, keys []domain.Key) ([]domain.Rating, error)
	// All returns every rating.
	All(ctx context.Context) ([]domain.Rating, error)
	// UpdateConfidence stores the new confidence of existing rows, all or nothing.
	UpdateConfidence(ctx context.Context, ratings []domain.Rating) error
}
