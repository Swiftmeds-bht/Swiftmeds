package clients

import (
	"context"

	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.PharmacyRegistryClient = (*PharmacyRegistryClient)(nil)
var _ ports.PharmacyOnboarding = (*PharmacyRegistryClient)(nil)

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

func (c *PharmacyRegistryClient) RegisterPharmacy(
	ctx context.Context,
	req *pharmacyregistryv1.RegisterPharmacyRequest,
) (*pharmacyregistryv1.RegisterPharmacyResponse, error) {
	return c.client.RegisterPharmacy(ctx, req)
}

func (c *PharmacyRegistryClient) ListPharmacies(
	ctx context.Context,
	req *pharmacyregistryv1.ListPharmaciesRequest,
) (*pharmacyregistryv1.ListPharmaciesResponse, error) {
	return c.client.ListPharmacies(ctx, req)
}

func (c *PharmacyRegistryClient) SetVerificationStatus(
	ctx context.Context,
	req *pharmacyregistryv1.SetVerificationStatusRequest,
) (*pharmacyregistryv1.SetVerificationStatusResponse, error) {
	return c.client.SetVerificationStatus(ctx, req)
}
