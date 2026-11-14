package ports

import (
	"context"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/domain"
)

type NotificationRepository interface {
	Create(context.Context, domain.Notification) (created bool, err error)
	ListByUser(context.Context, string, int, string) ([]domain.Notification, string, error)
}

// Delivery is the boundary for external channels. The current adapter is a
// deliberate no-op; providers can be added without changing the use cases.
type Delivery interface {
	SendEmail(context.Context, domain.Notification) error
	SendSMS(context.Context, domain.Notification) error
}

type Clock func() time.Time
