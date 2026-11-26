package ports

import (
	"context"

	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
)

// PricingClient sets and reads a pharmacy's prices (SCRUM-154).
type PricingClient interface {
	SetPrices(context.Context, *pricingv1.SetPricesRequest) (*pricingv1.SetPricesResponse, error)
	BatchGetPrices(context.Context, *pricingv1.BatchGetPricesRequest) (*pricingv1.BatchGetPricesResponse, error)
}
