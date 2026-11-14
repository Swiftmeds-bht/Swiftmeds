package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	notificationv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/notification/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/app"
)

const Queue = "notification.user-events"

type Channel interface {
	ExchangeDeclare(string, string, bool, bool, bool, bool, amqp.Table) error
	QueueDeclare(string, bool, bool, bool, bool, amqp.Table) (amqp.Queue, error)
	QueueBind(string, string, string, bool, amqp.Table) error
	Qos(int, int, bool) error
	ConsumeWithContext(context.Context, string, string, bool, bool, bool, bool, amqp.Table) (<-chan amqp.Delivery, error)
}

type Consumer struct{ consumer *messaging.Consumer }

func NewConsumer(ch Channel, deduper messaging.Deduper, svc *app.Service, log *slog.Logger) (*Consumer, error) {
	if svc == nil {
		return nil, errors.New("notification service is nil")
	}
	if log == nil {
		log = slog.Default()
	}
	c, err := messaging.NewConsumer(ch, messaging.ConsumerConfig{
		Queue: Queue, RoutingKeys: []string{"hold.confirmed", "hold.expired", "hold.rejected", "watchlist.alert"},
		Prefetch: 10, MaxAttempts: 3, RetryDelay: time.Second,
	}, deduper, func(ctx context.Context, d amqp.Delivery) error {
		if d.ContentType != messaging.ProtobufType {
			return fmt.Errorf("unexpected content type %q", d.ContentType)
		}
		eventID, userID, template, params, err := parseDelivery(d)
		if err != nil {
			return err
		}
		if _, err := svc.Send(ctx, eventID, userID, int32(notificationv1.Channel_CHANNEL_IN_APP), int32(template), params); err != nil {
			return err
		}
		log.InfoContext(ctx, "notification created", "event_id", eventID, "user_id", userID, "routing_key", d.RoutingKey)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create notification consumer: %w", err)
	}
	return &Consumer{consumer: c}, nil
}

func parseDelivery(d amqp.Delivery) (eventID, userID string, template notificationv1.Template, params map[string]string, err error) {
	params = map[string]string{}
	var metadata *eventsv1.EventMetadata
	switch d.RoutingKey {
	case "hold.confirmed":
		e := &eventsv1.HoldConfirmed{}
		if err = proto.Unmarshal(d.Body, e); err != nil {
			return "", "", notificationv1.Template_TEMPLATE_UNSPECIFIED, nil, err
		}
		metadata = e.GetMetadata()
		userID, template = e.GetUserId(), notificationv1.Template_TEMPLATE_HOLD_CONFIRMED
		params["hold_id"], params["pickup_code"] = e.GetHoldId(), e.GetPickupCode()
	case "hold.expired":
		e := &eventsv1.HoldExpired{}
		if err = proto.Unmarshal(d.Body, e); err != nil {
			return "", "", notificationv1.Template_TEMPLATE_UNSPECIFIED, nil, err
		}
		metadata = e.GetMetadata()
		userID, template = e.GetUserId(), notificationv1.Template_TEMPLATE_HOLD_EXPIRED
		params["hold_id"] = e.GetHoldId()
	case "hold.rejected":
		e := &eventsv1.HoldRejected{}
		if err = proto.Unmarshal(d.Body, e); err != nil {
			return "", "", notificationv1.Template_TEMPLATE_UNSPECIFIED, nil, err
		}
		metadata = e.GetMetadata()
		userID, template = e.GetUserId(), notificationv1.Template_TEMPLATE_HOLD_REJECTED
		params["hold_id"], params["reason"] = e.GetHoldId(), e.GetReason()
	case "watchlist.alert":
		e := &eventsv1.WatchlistAlert{}
		if err = proto.Unmarshal(d.Body, e); err != nil {
			return "", "", notificationv1.Template_TEMPLATE_UNSPECIFIED, nil, err
		}
		metadata = e.GetMetadata()
		userID, template = e.GetUserId(), notificationv1.Template_TEMPLATE_BACK_IN_STOCK
		params["product_id"], params["pharmacy_id"] = e.GetProductId(), e.GetPharmacyId()
		params["quantity_available"] = fmt.Sprint(e.GetQuantityAvailable())
	default:
		return "", "", notificationv1.Template_TEMPLATE_UNSPECIFIED, nil, fmt.Errorf("unexpected routing key %q", d.RoutingKey)
	}
	if metadata == nil || metadata.GetEventId() == "" {
		return "", "", notificationv1.Template_TEMPLATE_UNSPECIFIED, nil, errors.New("event metadata.event_id is required")
	}
	if d.MessageId == "" || d.MessageId != metadata.GetEventId() {
		return "", "", notificationv1.Template_TEMPLATE_UNSPECIFIED, nil, fmt.Errorf("message ID %q does not match metadata.event_id %q", d.MessageId, metadata.GetEventId())
	}
	eventID = metadata.GetEventId()
	return eventID, userID, template, params, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	if c == nil || c.consumer == nil {
		return errors.New("notification consumer is nil")
	}
	return c.consumer.Start(ctx)
}
