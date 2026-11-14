package noop

import (
	"context"

	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/ports"
)

var _ ports.Delivery = (*Delivery)(nil)

type Delivery struct{}

func (Delivery) SendEmail(context.Context, domain.Notification) error { return nil }
func (Delivery) SendSMS(context.Context, domain.Notification) error   { return nil }
