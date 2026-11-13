package clients

import (
	"context"

	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.PharmacyRegistryClient = (*PharmacyRegistryClient)(nil)

type PharmacyRegistryClient struct {
	client pharmacyregistryv1.PharmacyRegistryServiceClient
}

func NewPharmacyRegistryClient(client pharmacyregistryv1.PharmacyRegistryServiceClient) *PharmacyRegistryClient {
	return &PharmacyRegistryClient{client: client}
}

func (c *PharmacyRegistryClient) GetPharmacy(
	ctx context.Context,
	req *pharmacyregistryv1.GetPharmacyRequest,
) (*pharmacyregistryv1.GetPharmacyResponse, error) {
	return c.client.GetPharmacy(ctx, req)
}
