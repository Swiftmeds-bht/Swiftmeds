// Package ports defines the external services the app layer needs.
package ports

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/domain"
)

// Catalog provides product details and same-molecule candidates.
type Catalog interface {
	GetProduct(ctx context.Context, productID string) (domain.Product, error)
	ListProductsByMolecule(ctx context.Context, moleculeID, strength string) ([]domain.Product, error)
}
