package rabbitmq

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
)

var t0 = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

type expiredHolds struct{}

func (expiredHolds) ExpireHold(context.Context, string) (domain.HoldStatus, error) {
	return domain.HoldExpired, nil
}

func newService() *app.Service {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return app.NewService(memory.NewRepository(), expiredHolds{}, app.Config{}, func() time.Time { return t0 }, log)
}

func delivery(t *testing.T, event *eventsv1.HoldConfirmed) amqp.Delivery {
	t.Helper()
	body, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return amqp.Delivery{ContentType: messaging.ProtobufType, Body: body}
}

func TestAHoldConfirmedEventRecordsTheDeadlineSoTheSweepExpiresIt(t *testing.T) {
	svc := newService()
	event := &eventsv1.HoldConfirmed{HoldId: "h-1", ExpiresAt: timestamppb.New(t0)}

	if err := handleHoldConfirmed(context.Background(), delivery(t, event), svc); err != nil {
		t.Fatal(err)
	}

	res, err := svc.RunSweep(context.Background(), t0)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"h-1"}; !slices.Equal(res.Expired, want) {
		t.Errorf("the sweep expired %v, want %v", res.Expired, want)
	}
}

func TestTheSameEventTwiceIsHarmless(t *testing.T) {
	svc := newService()
	d := delivery(t, &eventsv1.HoldConfirmed{HoldId: "h-1", ExpiresAt: timestamppb.New(t0)})

	for range 2 {
		if err := handleHoldConfirmed(context.Background(), d, svc); err != nil {
			t.Fatal(err)
		}
	}

	res, _ := svc.RunSweep(context.Background(), t0)
	if len(res.Expired) != 1 {
		t.Errorf("expired = %v, want the hold once", res.Expired)
	}
}

func TestHandleHoldConfirmedRejectsBadDeliveries(t *testing.T) {
	svc := newService()
	wrongType := delivery(t, &eventsv1.HoldConfirmed{HoldId: "h-1", ExpiresAt: timestamppb.New(t0)})
	wrongType.ContentType = "application/json"

	cases := []struct {
		name     string
		delivery amqp.Delivery
	}{
		{"wrong content type", wrongType},
		{"body that is not a protobuf message", amqp.Delivery{ContentType: messaging.ProtobufType, Body: []byte{0xff, 0xff, 0xff}}},
		{"no expires_at", delivery(t, &eventsv1.HoldConfirmed{HoldId: "h-1"})},
		{"no hold_id", delivery(t, &eventsv1.HoldConfirmed{ExpiresAt: timestamppb.New(t0)})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := handleHoldConfirmed(context.Background(), c.delivery, svc); err == nil {
				t.Fatal("err = nil, want an error so the event is retried and then dead-lettered")
			}
		})
	}
}

func TestNewHoldConfirmedConsumerNeedsAServiceAndAChannel(t *testing.T) {
	if _, err := NewHoldConfirmedConsumer(nil, nil, nil); err == nil {
		t.Error("no service: err = nil, want an error")
	}
	if _, err := NewHoldConfirmedConsumer(nil, nil, newService()); err == nil {
		t.Error("no channel: err = nil, want an error")
	}
}

func TestStartOnANilConsumerFails(t *testing.T) {
	var c *HoldConfirmedConsumer

	if err := c.Start(context.Background()); err == nil {
		t.Fatal("err = nil, want an error")
	}
}
