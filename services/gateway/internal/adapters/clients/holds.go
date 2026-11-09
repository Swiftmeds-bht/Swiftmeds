package clients

import (
	"context"

	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.HoldsClient = (*HoldsClient)(nil)

type HoldsClient struct{ client holdsv1.HoldsServiceClient }

func NewHoldsClient(client holdsv1.HoldsServiceClient) *HoldsClient {
	return &HoldsClient{client: client}
}

func (c *HoldsClient) CreateHold(ctx context.Context, req *holdsv1.CreateHoldRequest) (*holdsv1.CreateHoldResponse, error) {
	return c.client.CreateHold(ctx, req)
}
func (c *HoldsClient) GetHold(ctx context.Context, req *holdsv1.GetHoldRequest) (*holdsv1.GetHoldResponse, error) {
	return c.client.GetHold(ctx, req)
}
func (c *HoldsClient) ListHolds(ctx context.Context, req *holdsv1.ListHoldsRequest) (*holdsv1.ListHoldsResponse, error) {
	return c.client.ListHolds(ctx, req)
}
func (c *HoldsClient) CancelHold(ctx context.Context, req *holdsv1.CancelHoldRequest) (*holdsv1.CancelHoldResponse, error) {
	return c.client.CancelHold(ctx, req)
}
func (c *HoldsClient) CollectHold(ctx context.Context, req *holdsv1.CollectHoldRequest) (*holdsv1.CollectHoldResponse, error) {
	return c.client.CollectHold(ctx, req)
}
