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

type Clock func() time.Time
