// Package clients holds the outbound gRPC adapters Feedback calls.
package clients

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/ports"
)

var _ ports.HoldReader = Holds{}

// Holds reads holds from the Holds service. Feedback only needs to know who
// owns a hold, where it was made, and whether it was collected.
type Holds struct{ C holdsv1.HoldsServiceClient }

func NewHolds(cc grpc.ClientConnInterface) Holds { return Holds{holdsv1.NewHoldsServiceClient(cc)} }

func (h Holds) GetHold(ctx context.Context, holdID string) (domain.Hold, error) {
	resp, err := h.C.GetHold(ctx, &holdsv1.GetHoldRequest{HoldId: holdID})
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			return domain.Hold{}, domain.ErrHoldNotFound
		case codes.Unavailable, codes.DeadlineExceeded:
			return domain.Hold{}, fmt.Errorf("%w: %v", domain.ErrHoldsUnavailable, err)
		default:
			return domain.Hold{}, fmt.Errorf("holds GetHold: %w", err)
		}
	}
	hold := resp.GetHold()
	if hold.GetHoldId() == "" {
		return domain.Hold{}, errors.Join(domain.ErrHoldNotFound, errors.New("holds returned an empty hold"))
	}
	return domain.Hold{
		ID:         hold.GetHoldId(),
		UserID:     hold.GetUserId(),
		PharmacyID: hold.GetPharmacyId(),
		ProductID:  hold.GetProductId(),
		Collected:  hold.GetStatus() == holdsv1.HoldStatus_HOLD_STATUS_COLLECTED,
	}, nil
}
