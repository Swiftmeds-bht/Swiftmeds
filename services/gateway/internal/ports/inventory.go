package ports

import (
	"context"

	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
)

// InventoryClient is the part of the Inventory service the pharmacy dashboard uses.
type InventoryClient interface {
	ListPharmacyStock(context.Context, *inventoryv1.ListPharmacyStockRequest) (*inventoryv1.ListPharmacyStockResponse, error)
	SetStock(context.Context, *inventoryv1.SetStockRequest) (*inventoryv1.SetStockResponse, error)
}
