package clients

import (
	"context"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.ProductLookup = (*CatalogClient)(nil)

// CatalogClient calls the Catalog service.
type CatalogClient struct {
	client catalogv1.CatalogServiceClient
}

func NewCatalogClient(client catalogv1.CatalogServiceClient) *CatalogClient {
	return &CatalogClient{client: client}
}

func (c *CatalogClient) GetProduct(ctx context.Context, req *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error) {
	return c.client.GetProduct(ctx, req)
}
