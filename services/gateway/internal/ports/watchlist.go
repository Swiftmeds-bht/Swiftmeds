package ports

import (
	"context"

	"google.golang.org/grpc"

	watchlistv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
)

type WatchlistClient interface {
	Subscribe(context.Context, *watchlistv1.SubscribeRequest) (*watchlistv1.SubscribeResponse, error)
	Unsubscribe(context.Context, *watchlistv1.UnsubscribeRequest) (*watchlistv1.UnsubscribeResponse, error)
	ListSubscriptions(context.Context, *watchlistv1.ListSubscriptionsRequest) (*watchlistv1.ListSubscriptionsResponse, error)
	// StreamAlerts stays open until ctx ends; cancelling ctx closes the gRPC stream.
	StreamAlerts(context.Context, *watchlistv1.StreamAlertsRequest) (grpc.ServerStreamingClient[watchlistv1.StockAlert], error)
}
