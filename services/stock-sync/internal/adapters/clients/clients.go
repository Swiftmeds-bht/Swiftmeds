package clients

import (
	"context"
	"errors"
	"fmt"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/ports"
	"google.golang.org/grpc"
)

type CatalogClient struct {
	client catalogv1.CatalogServiceClient
}

func NewCatalogClient(conn grpc.ClientConnInterface) *CatalogClient {
	return &CatalogClient{client: catalogv1.NewCatalogServiceClient(conn)}
}

func (c *CatalogClient) GetExistingProductIDs(
	ctx context.Context,
	productIDs []string,
) ([]string, error) {
	response, err := c.client.BatchGetProducts(ctx, &catalogv1.BatchGetProductsRequest{
		ProductIds: productIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("batch get catalog products: %w", err)
	}
	if response == nil {
		return nil, errors.New("catalog returned an empty product response")
	}

	existing := make([]string, 0, len(response.GetProducts()))
	for _, product := range response.GetProducts() {
		if product != nil {
			existing = append(existing, product.GetProductId())
		}
	}
	return existing, nil
}

type InventoryClient struct {
	client inventoryv1.InventoryServiceClient
}

func NewInventoryClient(conn grpc.ClientConnInterface) *InventoryClient {
	return &InventoryClient{client: inventoryv1.NewInventoryServiceClient(conn)}
}

func (c *InventoryClient) BulkSetStock(
	ctx context.Context,
	pharmacyID string,
	updates []domain.StockUpdate,
	updatedByUserID string,
) (domain.BulkStockResult, error) {
	protoUpdates := make([]*inventoryv1.StockUpdate, 0, len(updates))
	for _, update := range updates {
		protoUpdates = append(protoUpdates, &inventoryv1.StockUpdate{
			ProductId:      update.ProductID,
			QuantityOnHand: update.QuantityOnHand,
		})
	}

	response, err := c.client.BulkSetStock(ctx, &inventoryv1.BulkSetStockRequest{
		PharmacyId:      pharmacyID,
		Updates:         protoUpdates,
		UpdatedByUserId: updatedByUserID,
	})
	if err != nil {
		return domain.BulkStockResult{}, fmt.Errorf("bulk set inventory stock: %w", err)
	}
	if response == nil {
		return domain.BulkStockResult{}, errors.New("inventory returned an empty bulk response")
	}

	result := domain.BulkStockResult{
		UpdatedCount: response.GetUpdatedCount(),
		Errors:       make([]domain.StockUpdateError, 0, len(response.GetErrors())),
	}
	for _, rowError := range response.GetErrors() {
		if rowError == nil {
			continue
		}
		result.Errors = append(result.Errors, domain.StockUpdateError{
			ProductID: rowError.GetProductId(),
			Message:   rowError.GetMessage(),
		})
	}
	return result, nil
}

type PricingClient struct {
	client pricingv1.PricingServiceClient
}

func NewPricingClient(conn grpc.ClientConnInterface) *PricingClient {
	return &PricingClient{client: pricingv1.NewPricingServiceClient(conn)}
}

func (c *PricingClient) SetPrices(
	ctx context.Context,
	pharmacyID string,
	updates []domain.PriceUpdate,
) error {
	protoUpdates := make([]*pricingv1.PriceUpdate, 0, len(updates))
	for _, update := range updates {
		protoUpdates = append(protoUpdates, &pricingv1.PriceUpdate{
			ProductId:     update.ProductID,
			PriceChhertum: update.PriceChhertum,
		})
	}

	response, err := c.client.SetPrices(ctx, &pricingv1.SetPricesRequest{
		PharmacyId: pharmacyID,
		Updates:    protoUpdates,
	})
	if err != nil {
		return fmt.Errorf("set pricing prices: %w", err)
	}
	if response == nil {
		return errors.New("pricing returned an empty response")
	}
	return nil
}

var (
	_ ports.Catalog   = (*CatalogClient)(nil)
	_ ports.Inventory = (*InventoryClient)(nil)
	_ ports.Pricing   = (*PricingClient)(nil)
)
