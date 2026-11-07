package clients

import (
	"context"

	watchlistv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.WatchlistClient = (*WatchlistClient)(nil)

type WatchlistClient struct {
	client watchlistv1.WatchlistServiceClient
}

func NewWatchlistClient(client watchlistv1.WatchlistServiceClient) *WatchlistClient {
	return &WatchlistClient{client: client}
}
func (c *WatchlistClient) Subscribe(ctx context.Context, req *watchlistv1.SubscribeRequest) (*watchlistv1.SubscribeResponse, error) {
	return c.client.Subscribe(ctx, req)
}
func (c *WatchlistClient) Unsubscribe(ctx context.Context, req *watchlistv1.UnsubscribeRequest) (*watchlistv1.UnsubscribeResponse, error) {
	return c.client.Unsubscribe(ctx, req)
}
func (c *WatchlistClient) ListSubscriptions(ctx context.Context, req *watchlistv1.ListSubscriptionsRequest) (*watchlistv1.ListSubscriptionsResponse, error) {
	return c.client.ListSubscriptions(ctx, req)
}
