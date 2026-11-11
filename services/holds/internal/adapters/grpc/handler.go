// Package grpc is the inbound adapter: it translates HoldsService RPCs into
// app calls and domain errors into gRPC status codes. No business rules here.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// Handler implements holdsv1.HoldsServiceServer.
//
// The pickup code belongs to the patient. It is sent only where the patient
// asks for their own holds (GetHold, and ListHolds by user_id), and only while
// the hold is ACTIVE. Replies to pharmacy staff never carry it, so staff cannot
// collect a hold without the patient telling them the code.
type Handler struct {
	holdsv1.UnimplementedHoldsServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) CreateHold(ctx context.Context, req *holdsv1.CreateHoldRequest) (*holdsv1.CreateHoldResponse, error) {
	hold, err := h.svc.CreateHold(ctx, req.GetUserId(), req.GetPharmacyId(), req.GetProductId(), int(req.GetQuantity()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &holdsv1.CreateHoldResponse{Hold: toProto(hold, false)}, nil
}

func (h *Handler) GetHold(ctx context.Context, req *holdsv1.GetHoldRequest) (*holdsv1.GetHoldResponse, error) {
	hold, err := h.svc.GetHold(ctx, req.GetHoldId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &holdsv1.GetHoldResponse{Hold: toProto(hold, true)}, nil
}

func (h *Handler) ListHolds(ctx context.Context, req *holdsv1.ListHoldsRequest) (*holdsv1.ListHoldsResponse, error) {
	holdStatus, err := statusFromProto(req.GetStatus())
	if err != nil {
		return nil, toStatus(err)
	}
	res, err := h.svc.ListHolds(ctx, app.ListQuery{
		UserID:     req.GetUserId(),
		PharmacyID: req.GetPharmacyId(),
		Status:     holdStatus,
		PageSize:   int(req.GetPage().GetPageSize()),
		PageToken:  req.GetPage().GetPageToken(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	// A patient's own list shows the codes; a pharmacy's list does not.
	forPatient := req.GetUserId() != ""
	holds := make([]*holdsv1.Hold, 0, len(res.Holds))
	for _, hold := range res.Holds {
		holds = append(holds, toProto(hold, forPatient))
	}
	return &holdsv1.ListHoldsResponse{
		Holds: holds,
		Page:  &commonv1.PageResponse{NextPageToken: res.NextPageToken},
	}, nil
}

func (h *Handler) CancelHold(ctx context.Context, req *holdsv1.CancelHoldRequest) (*holdsv1.CancelHoldResponse, error) {
	hold, err := h.svc.CancelHold(ctx, req.GetHoldId(), req.GetUserId(), req.GetReason())
	if err != nil {
		return nil, toStatus(err)
	}
	return &holdsv1.CancelHoldResponse{Hold: toProto(hold, false)}, nil
}

func (h *Handler) CollectHold(ctx context.Context, req *holdsv1.CollectHoldRequest) (*holdsv1.CollectHoldResponse, error) {
	hold, err := h.svc.CollectHold(ctx, req.GetHoldId(), req.GetPharmacyId(), req.GetPickupCode())
	if err != nil {
		return nil, toStatus(err)
	}
	return &holdsv1.CollectHoldResponse{Hold: toProto(hold, false)}, nil
}

func (h *Handler) ExpireHold(ctx context.Context, req *holdsv1.ExpireHoldRequest) (*holdsv1.ExpireHoldResponse, error) {
	hold, err := h.svc.ExpireHold(ctx, req.GetHoldId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &holdsv1.ExpireHoldResponse{Hold: toProto(hold, false)}, nil
}

// toStatus maps a domain error to a gRPC status. Anything unexpected (a
// database error, for example) becomes a plain "internal error", so no
// internals leak to the caller; the server's interceptor logs the real error.
func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrMissingHoldID),
		errors.Is(err, domain.ErrMissingUserID),
		errors.Is(err, domain.ErrMissingPharmacyID),
		errors.Is(err, domain.ErrMissingProductID),
		errors.Is(err, domain.ErrMissingPickupCode),
		errors.Is(err, domain.ErrInvalidQuantity),
		errors.Is(err, domain.ErrInvalidListFilter),
		errors.Is(err, domain.ErrInvalidPageToken):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrLimitReached):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, domain.ErrWrongPickupCode),
		errors.Is(err, domain.ErrWrongPharmacy),
		errors.Is(err, domain.ErrNotHoldOwner):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrInvalidTransition),
		errors.Is(err, domain.ErrHoldExpired),
		errors.Is(err, domain.ErrNotYetExpired):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrInventoryUnavailable),
		errors.Is(err, domain.ErrRequestNotSent):
		// The hold was not changed (or, for CreateHold, was rejected): the patient can try again.
		return status.Error(codes.Unavailable, "stock service is not available, please try again")
	case errors.Is(err, domain.ErrConflict),
		errors.Is(err, domain.ErrPickupCodeTaken):
		// Someone else got there first. The caller may read the hold again and retry.
		return status.Error(codes.Aborted, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

// toProto converts a hold. The pickup code is included only when withCode is
// true and the hold is ACTIVE; the expiry time only when there is one.
func toProto(h domain.Hold, withCode bool) *holdsv1.Hold {
	out := &holdsv1.Hold{
		HoldId:          h.ID,
		UserId:          h.UserID,
		PharmacyId:      h.PharmacyID,
		ProductId:       h.ProductID,
		Quantity:        int32(h.Quantity),
		Status:          statusToProto(h.Status),
		CreatedAt:       timestamppb.New(h.CreatedAt),
		StatusChangedAt: timestamppb.New(h.StatusChangedAt),
		StatusReason:    h.StatusReason,
	}
	if !h.ExpiresAt.IsZero() {
		out.ExpiresAt = timestamppb.New(h.ExpiresAt)
	}
	if withCode && h.Status == domain.Active {
		out.PickupCode = h.PickupCode
	}
	return out
}

func statusToProto(s domain.Status) holdsv1.HoldStatus {
	switch s {
	case domain.Pending:
		return holdsv1.HoldStatus_HOLD_STATUS_PENDING
	case domain.Active:
		return holdsv1.HoldStatus_HOLD_STATUS_ACTIVE
	case domain.Collected:
		return holdsv1.HoldStatus_HOLD_STATUS_COLLECTED
	case domain.Expired:
		return holdsv1.HoldStatus_HOLD_STATUS_EXPIRED
	case domain.Cancelled:
		return holdsv1.HoldStatus_HOLD_STATUS_CANCELLED
	case domain.Rejected:
		return holdsv1.HoldStatus_HOLD_STATUS_REJECTED
	default:
		return holdsv1.HoldStatus_HOLD_STATUS_UNSPECIFIED
	}
}

// statusFromProto turns a list filter's status into a domain status. UNSPECIFIED
// means "any status" and gives an empty one.
func statusFromProto(s holdsv1.HoldStatus) (domain.Status, error) {
	switch s {
	case holdsv1.HoldStatus_HOLD_STATUS_UNSPECIFIED:
		return "", nil
	case holdsv1.HoldStatus_HOLD_STATUS_PENDING:
		return domain.Pending, nil
	case holdsv1.HoldStatus_HOLD_STATUS_ACTIVE:
		return domain.Active, nil
	case holdsv1.HoldStatus_HOLD_STATUS_COLLECTED:
		return domain.Collected, nil
	case holdsv1.HoldStatus_HOLD_STATUS_EXPIRED:
		return domain.Expired, nil
	case holdsv1.HoldStatus_HOLD_STATUS_CANCELLED:
		return domain.Cancelled, nil
	case holdsv1.HoldStatus_HOLD_STATUS_REJECTED:
		return domain.Rejected, nil
	default:
		return "", domain.ErrInvalidListFilter
	}
}
