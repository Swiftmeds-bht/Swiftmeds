// Package grpc is the inbound adapter: it translates ExpiryService RPCs into
// app calls and errors into gRPC status codes. No business rules here.
package grpc

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	expiryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/expiry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
)

// Handler implements expiryv1.ExpiryServiceServer.
type Handler struct {
	expiryv1.UnimplementedExpiryServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

// GetHoldDuration answers how long a new hold lasts. The same duration for
// every pharmacy and product for now; the request fields are for later rules.
func (h *Handler) GetHoldDuration(_ context.Context, _ *expiryv1.GetHoldDurationRequest) (*expiryv1.GetHoldDurationResponse, error) {
	return &expiryv1.GetHoldDurationResponse{Duration: durationpb.New(h.svc.HoldDuration())}, nil
}

// RunSweep expires every hold whose deadline is at or before as_of (now when
// as_of is left out). It is idempotent.
//
// If the sweep is cut short after it already expired some holds, those are
// returned with no error: the rest stay pending and the next sweep takes them.
// It fails only when it could not expire anything.
func (h *Handler) RunSweep(ctx context.Context, req *expiryv1.RunSweepRequest) (*expiryv1.RunSweepResponse, error) {
	var asOf time.Time
	if req.GetAsOf() != nil {
		asOf = req.GetAsOf().AsTime()
	}
	res, err := h.svc.RunSweep(ctx, asOf)
	if err != nil && len(res.Expired) == 0 {
		return nil, toStatus(err)
	}
	return &expiryv1.RunSweepResponse{ExpiredHoldIds: res.Expired}, nil
}

// toStatus maps an error to a gRPC status. Anything unexpected (a database
// error, for example) becomes a plain "internal error", so no internals leak;
// the server's interceptor logs the real error.
func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrHoldsUnavailable):
		return status.Error(codes.Unavailable, "holds service unavailable")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "sweep ran out of time")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "sweep was cancelled")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
