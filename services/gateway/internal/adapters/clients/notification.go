package clients

import (
	"context"

	notificationv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/notification/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.NotificationClient = (*NotificationClient)(nil)

type NotificationClient struct {
	client notificationv1.NotificationServiceClient
}

func NewNotificationClient(client notificationv1.NotificationServiceClient) *NotificationClient {
	return &NotificationClient{client: client}
}

func (c *NotificationClient) ListNotifications(ctx context.Context, req *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error) {
	return c.client.ListNotifications(ctx, req)
}
