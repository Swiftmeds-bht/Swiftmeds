package clients

import (
	"context"
	"errors"
	"fmt"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/ports"
	"google.golang.org/grpc"
)

// CatalogClient adapts the Catalog gRPC service to the Substitution ports.
type CatalogClient struct {
	client catalogv1.CatalogServiceClient
}

// NewCatalogClient creates a Catalog client using a gRPC connection.
func NewCatalogClient(conn grpc.ClientConnInterface) *CatalogClient {
	return &CatalogClient{
		client: catalogv1.NewCatalogServiceClient(conn),
	}
}

func (c *CatalogClient) GetProduct(
	ctx context.Context,
	productID string,
) (domain.Product, error) {
	response, err := c.client.GetProduct(ctx, &catalogv1.GetProductRequest{
		ProductId: productID,
	})
	if err != nil {
		return domain.Product{}, fmt.Errorf("get catalog product: %w", err)
	}
	if response == nil || response.GetProduct() == nil {
		return domain.Product{}, errors.New("catalog returned an empty product")
	}

	return mapProduct(response.GetProduct()), nil
}

func (c *CatalogClient) ListProductsByMolecule(
	ctx context.Context,
	moleculeID, strength string,
) ([]domain.Product, error) {
	response, err := c.client.ListProductsByMolecule(
		ctx,
		&catalogv1.ListProductsByMoleculeRequest{
			MoleculeId: moleculeID,
			Strength:   strength,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list catalog products by molecule: %w", err)
	}
	if response == nil {
		return nil, errors.New("catalog returned an empty product list response")
	}

	products := make([]domain.Product, 0, len(response.GetProducts()))
	for _, product := range response.GetProducts() {
		if product == nil {
			continue
		}
		products = append(products, mapProduct(product))
	}
	return products, nil
}

func mapProduct(product *catalogv1.Product) domain.Product {
	return domain.Product{
		ProductID:            product.GetProductId(),
		MoleculeID:           product.GetMoleculeId(),
		BrandName:            product.GetBrandName(),
		GenericName:          product.GetGenericName(),
		Strength:             product.GetStrength(),
		DosageForm:           product.GetDosageForm().String(),
		RequiresPrescription: product.GetRequiresPrescription(),
		Controlled:           product.GetControlled(),
	}
}

var _ ports.Catalog = (*CatalogClient)(nil)
