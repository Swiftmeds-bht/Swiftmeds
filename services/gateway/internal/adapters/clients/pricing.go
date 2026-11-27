package clients

import (
	"context"

	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.PricingClient = (*PricingClient)(nil)

// PricingClient calls the Pricing service.
type PricingClient struct {
	client pricingv1.PricingServiceClient
}

func NewPricingClient(client pricingv1.PricingServiceClient) *PricingClient {
	return &PricingClient{client: client}
}

func (c *PricingClient) SetPrices(ctx context.Context, req *pricingv1.SetPricesRequest) (*pricingv1.SetPricesResponse, error) {
	return c.client.SetPrices(ctx, req)
}

func (c *PricingClient) BatchGetPrices(ctx context.Context, req *pricingv1.BatchGetPricesRequest) (*pricingv1.BatchGetPricesResponse, error) {
	return c.client.BatchGetPrices(ctx, req)
}
