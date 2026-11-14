package clients

import (
	"context"

	analyticsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/analytics/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.AnalyticsClient = (*AnalyticsClient)(nil)

type AnalyticsClient struct {
	client analyticsv1.AnalyticsServiceClient
}

func NewAnalyticsClient(client analyticsv1.AnalyticsServiceClient) *AnalyticsClient {
	return &AnalyticsClient{client: client}
}

func (c *AnalyticsClient) GetUnmetDemand(
	ctx context.Context,
	req *analyticsv1.GetUnmetDemandRequest,
) (*analyticsv1.GetUnmetDemandResponse, error) {
	return c.client.GetUnmetDemand(ctx, req)
}
