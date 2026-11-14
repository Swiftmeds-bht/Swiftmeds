package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	notificationv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/notification/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/domain"
)

type Handler struct {
	notificationv1.UnimplementedNotificationServiceServer
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) SendNotification(ctx context.Context, req *notificationv1.SendNotificationRequest) (*notificationv1.SendNotificationResponse, error) {
	n, err := h.svc.Send(ctx, app.NewID(), req.GetUserId(), int32(req.GetChannel()), int32(req.GetTemplate()), req.GetParams())
	if err != nil {
		return nil, toStatus(err)
	}
	return &notificationv1.SendNotificationResponse{NotificationId: n.ID}, nil
}

func (h *Handler) ListNotifications(ctx context.Context, req *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error) {
	items, next, err := h.svc.List(ctx, req.GetUserId(), int(req.GetPage().GetPageSize()), req.GetPage().GetPageToken())
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*notificationv1.Notification, 0, len(items))
	for _, n := range items {
		item := &notificationv1.Notification{
			NotificationId: n.ID, UserId: n.UserID, Channel: notificationv1.Channel(n.Channel),
			Template: notificationv1.Template(n.Template), Params: n.Params,
			CreatedAt: timestamppb.New(n.CreatedAt),
		}
		if !n.ReadAt.IsZero() {
			item.ReadAt = timestamppb.New(n.ReadAt)
		}
		out = append(out, item)
	}
	return &notificationv1.ListNotificationsResponse{Notifications: out, Page: &commonv1.PageResponse{NextPageToken: next}}, nil
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrMissingUserID), errors.Is(err, domain.ErrMissingEventID),
		errors.Is(err, domain.ErrInvalidChannel), errors.Is(err, domain.ErrInvalidTemplate):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
