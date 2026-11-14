package ports

import (
	"context"

	notificationv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/notification/v1"
)

type NotificationClient interface {
	ListNotifications(context.Context, *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error)
}
