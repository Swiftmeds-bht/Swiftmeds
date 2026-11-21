package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
)

var (
	now      = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	occurred = now.Add(-time.Minute)
	clock    = func() time.Time { return now }
)

type recordingHandler struct {
	got     []domain.Feedback
	applied bool
	err     error
}

func (h *recordingHandler) HandleFeedbackSubmitted(_ context.Context, f domain.Feedback) (bool, error) {
	h.got = append(h.got, f)
	return h.applied, h.err
}

func delivery(t *testing.T, ev *eventsv1.FeedbackSubmitted) amqp.Delivery {
	t.Helper()
	body, err := proto.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	return amqp.Delivery{ContentType: messaging.ProtobufType, Body: body, MessageId: "ev-1"}
}

func TestHandleDecodesTheEvent(t *testing.T) {
	for _, applied := range []bool{true, false} {
		h := &recordingHandler{applied: applied}
		d := delivery(t, &eventsv1.FeedbackSubmitted{
			Metadata:   &eventsv1.EventMetadata{EventId: "ev-1", OccurredAt: timestamppb.New(occurred), Producer: "feedback"},
			FeedbackId: "fb-1",
			HoldId:     "hold-1",
			PharmacyId: "ph-1",
			ProductId:  "pr-1",
			WasInStock: true,
			Rating:     4,
		})
		if err := handle(context.Background(), d, h, clock); err != nil {
			t.Fatal(err)
		}
		want := domain.Feedback{ID: "fb-1", PharmacyID: "ph-1", WasInStock: true, OccurredAt: occurred}
		if len(h.got) != 1 || h.got[0] != want {
			t.Errorf("handler got %+v, want %+v", h.got, want)
		}
	}
}

func TestHandleUsesTheClockWithoutOccurredAt(t *testing.T) {
	h := &recordingHandler{}
	if err := handle(context.Background(), delivery(t, &eventsv1.FeedbackSubmitted{FeedbackId: "fb-1", PharmacyId: "ph-1"}), h, clock); err != nil {
		t.Fatal(err)
	}
	if !h.got[0].OccurredAt.Equal(now) || h.got[0].WasInStock {
		t.Errorf("got %+v, want the clock's time and 'not in stock'", h.got[0])
	}
}

func TestHandleRejectsBadDeliveries(t *testing.T) {
	h := &recordingHandler{}
	if err := handle(context.Background(), amqp.Delivery{ContentType: "application/json", Body: []byte("{}")}, h, clock); err == nil {
		t.Error("accepted a JSON delivery")
	}
	if err := handle(context.Background(), amqp.Delivery{ContentType: messaging.ProtobufType, Body: []byte{0xff, 0xff}}, h, clock); err == nil {
		t.Error("accepted a body that is not a FeedbackSubmitted")
	}
	if len(h.got) != 0 {
		t.Errorf("handler called for bad deliveries: %+v", h.got)
	}
}

func TestHandlePassesHandlerErrorsOnForRetry(t *testing.T) {
	h := &recordingHandler{err: errors.New("database unavailable")}
	if err := handle(context.Background(), delivery(t, &eventsv1.FeedbackSubmitted{FeedbackId: "fb-1", PharmacyId: "ph-1"}), h, clock); err == nil {
		t.Fatal("want the handler's error, so pkg/messaging retries and then dead-letters")
	}
}

// fakeChannel records what the consumer declares.
type fakeChannel struct {
	queues []string
	binds  [][2]string
}

func (c *fakeChannel) ExchangeDeclare(string, string, bool, bool, bool, bool, amqp.Table) error {
	return nil
}

func (c *fakeChannel) QueueDeclare(name string, _, _, _, _ bool, _ amqp.Table) (amqp.Queue, error) {
	c.queues = append(c.queues, name)
	return amqp.Queue{Name: name}, nil
}

func (c *fakeChannel) QueueBind(name, key, _ string, _ bool, _ amqp.Table) error {
	c.binds = append(c.binds, [2]string{name, key})
	return nil
}

func (c *fakeChannel) Qos(int, int, bool) error { return nil }

func (c *fakeChannel) ConsumeWithContext(context.Context, string, string, bool, bool, bool, bool, amqp.Table) (<-chan amqp.Delivery, error) {
	return make(chan amqp.Delivery), nil
}

// noDedup claims every event, so every delivery reaches the handler.
type noDedup struct{}

func (noDedup) Claim(context.Context, string) (messaging.Claim, bool, error) {
	return noClaim{}, true, nil
}

// noClaim is the claim noDedup hands out; it records nothing.
type noClaim struct{}

func (noClaim) Commit(context.Context) error  { return nil }
func (noClaim) Release(context.Context) error { return nil }

func TestNewConsumerBindsFeedbackSubmitted(t *testing.T) {
	ch := &fakeChannel{}
	c, err := NewFeedbackSubmittedConsumer(ch, noDedup{}, &recordingHandler{}, clock)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range ch.binds {
		if b == [2]string{Queue, RoutingKey} {
			found = true
		}
	}
	if !found {
		t.Errorf("binds = %v, want %s bound to %s", ch.binds, Queue, RoutingKey)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Start(ctx); err != nil {
		t.Errorf("Start after cancel: %v", err)
	}
}

func TestNewConsumerNeedsAHandler(t *testing.T) {
	if _, err := NewFeedbackSubmittedConsumer(&fakeChannel{}, noDedup{}, nil, clock); err == nil {
		t.Fatal("accepted a nil handler")
	}
	if _, err := NewFeedbackSubmittedConsumer(nil, noDedup{}, &recordingHandler{}, clock); err == nil {
		t.Fatal("accepted a nil channel")
	}
}
