package clients

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	expiryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/expiry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.HoldSweeper = (*ExpiryClient)(nil)

// ExpiryClient calls the Expiry service.
type ExpiryClient struct {
	client expiryv1.ExpiryServiceClient
}

func NewExpiryClient(client expiryv1.ExpiryServiceClient) *ExpiryClient {
	return &ExpiryClient{client: client}
}

// RunSweep asks Expiry to expire every hold due at asOf. A gRPC error is
// returned as it is, so the HTTP layer can map its status code.
func (e *ExpiryClient) RunSweep(ctx context.Context, asOf time.Time) ([]string, error) {
	resp, err := e.client.RunSweep(ctx, &expiryv1.RunSweepRequest{AsOf: timestamppb.New(asOf)})
	if err != nil {
		return nil, err
	}
	return resp.GetExpiredHoldIds(), nil
}
