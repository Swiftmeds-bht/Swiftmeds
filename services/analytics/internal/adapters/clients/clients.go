package clients

import (
	"context"
	"errors"
	"fmt"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
	pharmacyv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/ports"
	"google.golang.org/grpc"
)

var _ ports.Catalog = Catalog{}
var _ ports.PharmacyRegistry = Registry{}

type Catalog struct {
	client catalogv1.CatalogServiceClient
}

func NewCatalog(conn grpc.ClientConnInterface) Catalog {
	return Catalog{catalogv1.NewCatalogServiceClient(conn)}
}
func (c Catalog) GetProduct(ctx context.Context, id string) (string, error) {
	resp, err := c.client.GetProduct(ctx, &catalogv1.GetProductRequest{ProductId: id})
	if err != nil {
		return "", fmt.Errorf("catalog GetProduct: %w", err)
	}
	if resp.GetProduct() == nil || resp.GetProduct().GetProductId() == "" {
		return "", errors.New("catalog returned no product")
	}
	p := resp.GetProduct()
	if p.GetBrandName() != "" {
		return p.GetBrandName(), nil
	}
	return p.GetGenericName(), nil
}

type Registry struct {
	client pharmacyv1.PharmacyRegistryServiceClient
}

func NewRegistry(conn grpc.ClientConnInterface) Registry {
	return Registry{pharmacyv1.NewPharmacyRegistryServiceClient(conn)}
}
func (r Registry) GetPharmacy(ctx context.Context, id string) (string, error) {
	resp, err := r.client.GetPharmacy(ctx, &pharmacyv1.GetPharmacyRequest{PharmacyId: id})
	if err != nil {
		return "", fmt.Errorf("pharmacy registry GetPharmacy: %w", err)
	}
	if resp.GetPharmacy() == nil || resp.GetPharmacy().GetPharmacyId() == "" {
		return "", errors.New("pharmacy registry returned no pharmacy")
	}
	return resp.GetPharmacy().GetDzongkhag(), nil
}
