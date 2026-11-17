package clients

import (
	"context"

	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.InventoryClient = (*InventoryClient)(nil)

type InventoryClient struct {
	client inventoryv1.InventoryServiceClient
}

func NewInventoryClient(client inventoryv1.InventoryServiceClient) *InventoryClient {
	return &InventoryClient{client: client}
}

func (c *InventoryClient) ListPharmacyStock(ctx context.Context, req *inventoryv1.ListPharmacyStockRequest) (*inventoryv1.ListPharmacyStockResponse, error) {
	return c.client.ListPharmacyStock(ctx, req)
}

func (c *InventoryClient) SetStock(ctx context.Context, req *inventoryv1.SetStockRequest) (*inventoryv1.SetStockResponse, error) {
	return c.client.SetStock(ctx, req)
}
