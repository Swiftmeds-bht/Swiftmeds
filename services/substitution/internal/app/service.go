// Package app contains the Substitution use cases.
package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/ports"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// Result is the response for one source product.
type Result struct {
	MoleculeID  string
	Substitutes []domain.Substitute
}

// Service implements Substitution use cases.
type Service struct {
	catalog ports.Catalog
}

// NewService wires the Catalog dependency.
func NewService(catalog ports.Catalog) *Service {
	return &Service{catalog: catalog}
}

// GetSubstitutes returns same-molecule candidates, classifying each by strength
// and dosage-form match. An empty strength lookup lets the service flag differences.
func (s *Service) GetSubstitutes(
	ctx context.Context,
	productID string,
	limit int,
) (Result, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return Result{}, domain.ErrMissingProductID
	}

	source, err := s.catalog.GetProduct(ctx, productID)
	if err != nil {
		return Result{}, fmt.Errorf("get source product: %w", err)
	}

	moleculeID := strings.TrimSpace(source.MoleculeID)
	if moleculeID == "" {
		return Result{}, domain.ErrMissingMoleculeID
	}

	result := Result{
		MoleculeID:  moleculeID,
		Substitutes: []domain.Substitute{},
	}
	if source.Controlled {
		return result, nil
	}

	candidates, err := s.catalog.ListProductsByMolecule(ctx, moleculeID, "")
	if err != nil {
		return Result{}, fmt.Errorf("list products by molecule: %w", err)
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		substitute, ok := domain.BuildSubstitute(source, candidate)
		if !ok {
			continue
		}

		key := strings.ToLower(substitute.ProductID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result.Substitutes = append(result.Substitutes, substitute)
	}

	sort.SliceStable(result.Substitutes, func(i, j int) bool {
		a, b := result.Substitutes[i], result.Substitutes[j]
		if a.MatchType != b.MatchType {
			return a.MatchType == domain.MatchTypeExact
		}
		if !strings.EqualFold(a.BrandName, b.BrandName) {
			return strings.ToLower(a.BrandName) < strings.ToLower(b.BrandName)
		}
		return a.ProductID < b.ProductID
	})

	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if len(result.Substitutes) > limit {
		result.Substitutes = result.Substitutes[:limit]
	}

	return result, nil
}
