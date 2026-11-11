// Package grpc is the inbound adapter: it translates FeedbackService RPCs into
// app calls and domain errors into gRPC status codes. No business rules here.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	feedbackv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/feedback/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
)

// Handler implements feedbackv1.FeedbackServiceServer.
type Handler struct {
	feedbackv1.UnimplementedFeedbackServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) SubmitFeedback(ctx context.Context, req *feedbackv1.SubmitFeedbackRequest) (*feedbackv1.SubmitFeedbackResponse, error) {
	f, err := h.svc.Submit(ctx, app.SubmitInput{
		HoldID:     req.GetHoldId(),
		UserID:     req.GetUserId(),
		WasInStock: req.GetWasInStock(),
		Rating:     int(req.GetRating()),
		Comment:    req.GetComment(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &feedbackv1.SubmitFeedbackResponse{Feedback: toProto(f)}, nil
}

func (h *Handler) ListPharmacyFeedback(ctx context.Context, req *feedbackv1.ListPharmacyFeedbackRequest) (*feedbackv1.ListPharmacyFeedbackResponse, error) {
	res, err := h.svc.ListPharmacyFeedback(ctx, app.ListQuery{
		PharmacyID: req.GetPharmacyId(),
		PageSize:   int(req.GetPage().GetPageSize()),
		PageToken:  req.GetPage().GetPageToken(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	items := make([]*feedbackv1.Feedback, 0, len(res.Feedback))
	for _, f := range res.Feedback {
		items = append(items, toProto(f))
	}
	return &feedbackv1.ListPharmacyFeedbackResponse{
		Feedback: items,
		Page:     &commonv1.PageResponse{NextPageToken: res.NextPageToken},
	}, nil
}

// toStatus maps a domain error to a gRPC status. Anything unexpected (a
// database error, for example) becomes a plain "internal error", so no
// internals leak to the caller; the server's interceptor logs the real error.
func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrMissingHoldID),
		errors.Is(err, domain.ErrMissingUserID),
		errors.Is(err, domain.ErrMissingPharmacyID),
		errors.Is(err, domain.ErrInvalidRating),
		errors.Is(err, domain.ErrCommentTooLong),
		errors.Is(err, domain.ErrInvalidPageToken):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrHoldNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrNotHoldOwner):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrHoldNotCollected):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrAlreadySubmitted):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrHoldsUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, domain.ErrPublishFailed):
		// Nothing was saved, so the caller may simply try again.
		return status.Error(codes.Unavailable, "could not publish the feedback event; nothing was saved, try again")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func toProto(f domain.Feedback) *feedbackv1.Feedback {
	return &feedbackv1.Feedback{
		FeedbackId: f.ID,
		HoldId:     f.HoldID,
		PharmacyId: f.PharmacyID,
		ProductId:  f.ProductID,
		WasInStock: f.WasInStock,
		Rating:     int32(f.Rating),
		Comment:    f.Comment,
		CreatedAt:  timestamppb.New(f.CreatedAt),
	}
}
