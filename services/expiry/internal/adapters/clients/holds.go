// Package clients holds the outbound adapters: gRPC clients for other services.
package clients

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/ports"
)

var _ ports.HoldExpirer = (*HoldsClient)(nil)

// HoldsClient calls the Holds service.
type HoldsClient struct {
	client holdsv1.HoldsServiceClient
}

func NewHoldsClient(conn grpc.ClientConnInterface) *HoldsClient {
	return &HoldsClient{client: holdsv1.NewHoldsServiceClient(conn)}
}

// ExpireHold asks Holds to expire the hold. Holds does nothing to a hold that
// is not ACTIVE, so calling it again is safe. Holds' answers are turned into
// the domain errors the sweep understands:
//
//	NOT_FOUND            -> domain.ErrHoldNotFound
//	FAILED_PRECONDITION  -> domain.ErrNotYetExpired (Holds' clock is a moment behind)
//	UNAVAILABLE, DEADLINE_EXCEEDED, CANCELED -> domain.ErrHoldsUnavailable
func (c *HoldsClient) ExpireHold(ctx context.Context, holdID string) (domain.HoldStatus, error) {
	res, err := c.client.ExpireHold(ctx, &holdsv1.ExpireHoldRequest{HoldId: holdID})
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			return "", domain.ErrHoldNotFound
		case codes.FailedPrecondition:
			return "", domain.ErrNotYetExpired
		case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
			return "", fmt.Errorf("%w: %s", domain.ErrHoldsUnavailable, status.Convert(err).Message())
		}
		return "", fmt.Errorf("holds.ExpireHold: %w", err)
	}
	if res.GetHold() == nil {
		return "", errors.New("holds.ExpireHold returned no hold")
	}
	return statusFromProto(res.GetHold().GetStatus()), nil
}

func statusFromProto(s holdsv1.HoldStatus) domain.HoldStatus {
	switch s {
	case holdsv1.HoldStatus_HOLD_STATUS_PENDING:
		return domain.HoldPending
	case holdsv1.HoldStatus_HOLD_STATUS_ACTIVE:
		return domain.HoldActive
	case holdsv1.HoldStatus_HOLD_STATUS_COLLECTED:
		return domain.HoldCollected
	case holdsv1.HoldStatus_HOLD_STATUS_EXPIRED:
		return domain.HoldExpired
	case holdsv1.HoldStatus_HOLD_STATUS_CANCELLED:
		return domain.HoldCancelled
	case holdsv1.HoldStatus_HOLD_STATUS_REJECTED:
		return domain.HoldRejected
	default:
		return ""
	}
}
