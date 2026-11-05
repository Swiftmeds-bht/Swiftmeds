// Package clients holds the outbound adapters: gRPC clients for other services.
package clients

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	freshnessv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/freshness/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.DecayApplier = (*FreshnessClient)(nil)

// FreshnessClient calls the Freshness service.
type FreshnessClient struct {
	client freshnessv1.FreshnessServiceClient
}

func NewFreshnessClient(client freshnessv1.FreshnessServiceClient) *FreshnessClient {
	return &FreshnessClient{client: client}
}

// ApplyDecay asks Freshness to decay every stored confidence to asOf. A gRPC
// error is returned as it is, so the HTTP layer can map its status code
// (Unavailable becomes 503, and so on).
func (f *FreshnessClient) ApplyDecay(ctx context.Context, asOf time.Time) (int, error) {
	resp, err := f.client.ApplyDecay(ctx, &freshnessv1.ApplyDecayRequest{AsOf: timestamppb.New(asOf)})
	if err != nil {
		return 0, err
	}
	return int(resp.GetUpdatedCount()), nil
}
