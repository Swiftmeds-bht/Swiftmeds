// Package ports defines the interfaces the Catalog app needs from adapters.
package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/domain"
)

// ProductRepository stores and searches catalog products.
type ProductRepository interface {
	// SearchByName matches brand or generic names and never returns controlled products.
	SearchByName(ctx context.Context, query string, limit int) ([]domain.Product, error)

	// GetByID returns one product.
	GetByID(ctx context.Context, productID string) (domain.Product, error)

	// GetMany returns existing products; unknown IDs are omitted.
	GetMany(ctx context.Context, productIDs []string) ([]domain.Product, error)

	// ListByMolecule returns products for a molecule, optionally restricted to one strength.
	// Controlled products are excluded.
	ListByMolecule(ctx context.Context, moleculeID, strength string) ([]domain.Product, error)

	// Create inserts a product and returns it with its generated ID.
	Create(ctx context.Context, product domain.Product) (domain.Product, error)
}
