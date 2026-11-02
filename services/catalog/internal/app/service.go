// Package app contains the Catalog use cases.
package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/catalog/internal/ports"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 100
)

// Service implements the Catalog use cases.
type Service struct {
	repo ports.ProductRepository
}

func NewService(repo ports.ProductRepository) *Service {
	return &Service{repo: repo}
}

// ResolveName searches brand and generic names. A zero or negative limit uses
// the default; oversized limits are capped.
func (s *Service) ResolveName(
	ctx context.Context,
	query string,
	limit int,
) ([]domain.Product, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, domain.ErrMissingQuery
	}

	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	products, err := s.repo.SearchByName(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search products: %w", err)
	}
	return products, nil
}

func (s *Service) GetProduct(
	ctx context.Context,
	productID string,
) (domain.Product, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return domain.Product{}, domain.ErrMissingProductID
	}

	product, err := s.repo.GetByID(ctx, productID)
	if err != nil {
		return domain.Product{}, fmt.Errorf("get product: %w", err)
	}
	return product, nil
}

// BatchGetProducts omits unknown IDs and removes duplicate IDs from the request.
func (s *Service) BatchGetProducts(
	ctx context.Context,
	productIDs []string,
) ([]domain.Product, error) {
	ids := make([]string, 0, len(productIDs))
	seen := make(map[string]struct{}, len(productIDs))
	for _, id := range productIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return []domain.Product{}, nil
	}

	products, err := s.repo.GetMany(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("batch get products: %w", err)
	}
	return products, nil
}

func (s *Service) ListProductsByMolecule(
	ctx context.Context,
	moleculeID string,
	strength string,
) ([]domain.Product, error) {
	moleculeID = strings.TrimSpace(moleculeID)
	if moleculeID == "" {
		return nil, domain.ErrMissingMoleculeID
	}

	strength = strings.ToLower(strings.Join(strings.Fields(strength), ""))
	products, err := s.repo.ListByMolecule(ctx, moleculeID, strength)
	if err != nil {
		return nil, fmt.Errorf("list products by molecule: %w", err)
	}
	return products, nil
}

func (s *Service) CreateProduct(
	ctx context.Context,
	input domain.ProductInput,
) (domain.Product, error) {
	product, err := domain.NewProduct(input)
	if err != nil {
		return domain.Product{}, fmt.Errorf("validate product: %w", err)
	}

	created, err := s.repo.Create(ctx, product)
	if err != nil {
		return domain.Product{}, fmt.Errorf("create product: %w", err)
	}
	return created, nil
}
