package rabbitmq

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// ---- handleStockEvent: decoding and dispatching -------------------------------------------------

type fakeHandler struct {
	reserved    []string
	unavailable []string
	outcome     app.Outcome
	err         error
}

func (f *fakeHandler) HandleStockReserved(_ context.Context, holdID string) (app.Outcome, error) {
	f.reserved = append(f.reserved, holdID)
	return f.outcome, f.err
}

func (f *fakeHandler) HandleStockUnavailable(_ context.Context, holdID string) (app.Outcome, error) {
	f.unavailable = append(f.unavailable, holdID)
	return f.outcome, f.err
}

func delivery(t *testing.T, key string, event proto.Message) amqp.Delivery {
	t.Helper()
	body, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return amqp.Delivery{RoutingKey: key, ContentType: messaging.ProtobufType, Body: body, MessageId: "m-1"}
}

func stockReserved(holdID string) *eventsv1.StockReserved {
	return &eventsv1.StockReserved{HoldId: holdID, PharmacyId: "ph-1", ProductId: "med-1", Quantity: 2}
}

func stockUnavailable(holdID string) *eventsv1.StockUnavailable {
	return &eventsv1.StockUnavailable{HoldId: holdID, RequestedQuantity: 5, AvailableQuantity: 1}
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)) }

func TestStockReservedIsPassedToTheSaga(t *testing.T) {
	h := &fakeHandler{outcome: app.OutcomeConfirmed}

	err := handleStockEvent(context.Background(), delivery(t, "stock.reserved", stockReserved("h-1")), h, quietLog())

	if err != nil {
		t.Fatal(err)
	}
	if len(h.reserved) != 1 || h.reserved[0] != "h-1" || len(h.unavailable) != 0 {
		t.Errorf("reserved %v, unavailable %v, want only reserved [h-1]", h.reserved, h.unavailable)
	}
}

func TestStockUnavailableIsPassedToTheSaga(t *testing.T) {
	h := &fakeHandler{outcome: app.OutcomeRejected}

	err := handleStockEvent(context.Background(), delivery(t, "stock.unavailable", stockUnavailable("h-9")), h, quietLog())

	if err != nil {
		t.Fatal(err)
	}
	if len(h.unavailable) != 1 || h.unavailable[0] != "h-9" || len(h.reserved) != 0 {
		t.Errorf("reserved %v, unavailable %v, want only unavailable [h-9]", h.reserved, h.unavailable)
	}
}

func TestTheOutcomeIsLogged(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	d := delivery(t, "stock.reserved", stockReserved("h-1"))
	d.MessageId = "m-42"

	_ = handleStockEvent(context.Background(), d, &fakeHandler{outcome: app.OutcomeReleased}, log)

	for _, want := range []string{"hold_id=h-1", "outcome=released", "message_id=m-42", "routing_key=stock.reserved"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log %q does not contain %q", buf.String(), want)
		}
	}
}

func TestEventsThatCannotBeUnderstoodFail(t *testing.T) {
	badType := delivery(t, "stock.reserved", stockReserved("h-1"))
	badType.ContentType = "application/json"
	garbageReserved := amqp.Delivery{RoutingKey: "stock.reserved", ContentType: messaging.ProtobufType, Body: []byte{0xff, 0xff, 0xff}}
	garbageUnavailable := amqp.Delivery{RoutingKey: "stock.unavailable", ContentType: messaging.ProtobufType, Body: []byte{0xff, 0xff, 0xff}}
	unknownKey := delivery(t, "stock.increased", stockReserved("h-1"))

	for name, d := range map[string]amqp.Delivery{
		"wrong content type":          badType,
		"stock.reserved, garbage":     garbageReserved,
		"stock.unavailable, garbage":  garbageUnavailable,
		"a routing key we do not use": unknownKey,
	} {
		t.Run(name, func(t *testing.T) {
			h := &fakeHandler{}

			if err := handleStockEvent(context.Background(), d, h, quietLog()); err == nil {
				t.Fatal("err = nil, want an error (the event is retried, then dead-lettered)")
			}
			if len(h.reserved)+len(h.unavailable) != 0 {
				t.Error("the saga was called for an event that could not be understood")
			}
		})
	}
}

func TestSagaErrorsAreReturnedSoTheEventIsRetried(t *testing.T) {
	h := &fakeHandler{err: domain.ErrInventoryUnavailable}

	err := handleStockEvent(context.Background(), delivery(t, "stock.reserved", stockReserved("h-1")), h, quietLog())

	if !errors.Is(err, domain.ErrInventoryUnavailable) {
		t.Fatalf("err = %v, want the saga's error", err)
	}
}

func TestNewStockConsumerNeedsAHandler(t *testing.T) {
	if _, err := NewStockConsumer(&fakeChannel{}, newMemoryDeduper(), nil, nil); err == nil {
		t.Error("err = nil, want an error")
	}
}

// ---- the real consumer, deduplication and Holds' own logic together ----------------------------

type memoryDeduper struct {
	mu   sync.Mutex
	seen map[string]bool
}

func newMemoryDeduper() *memoryDeduper { return &memoryDeduper{seen: map[string]bool{}} }

func (d *memoryDeduper) IsProcessed(_ context.Context, id string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.seen[id], nil
}

func (d *memoryDeduper) MarkProcessed(_ context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen[id] = true
	return nil
}

// ack records how each delivery was settled.
type ack struct {
	mu    sync.Mutex
	acked []uint64
	nacks []uint64
}

func (a *ack) Ack(tag uint64, _ bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.acked = append(a.acked, tag)
	return nil
}

func (a *ack) Nack(tag uint64, _, _ bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nacks = append(a.nacks, tag)
	return nil
}

func (a *ack) Reject(uint64, bool) error { return nil }

// fakeChannel hands the consumer a fixed list of deliveries.
type fakeChannel struct {
	queue      string
	bindings   []string
	deliveries chan amqp.Delivery
}

func (c *fakeChannel) ExchangeDeclare(string, string, bool, bool, bool, bool, amqp.Table) error {
	return nil
}

func (c *fakeChannel) QueueDeclare(name string, _, _, _, _ bool, _ amqp.Table) (amqp.Queue, error) {
	if !strings.HasSuffix(name, ".dlq") {
		c.queue = name
	}
	return amqp.Queue{Name: name}, nil
}

// QueueBind records only the bindings on the events exchange (the dead-letter queue has its own).
func (c *fakeChannel) QueueBind(_, key, exchange string, _ bool, _ amqp.Table) error {
	if exchange == messaging.ExchangeName {
		c.bindings = append(c.bindings, key)
	}
	return nil
}

func (c *fakeChannel) Qos(int, int, bool) error { return nil }

func (c *fakeChannel) ConsumeWithContext(_ context.Context, _, _ string, _, _, _, _ bool, _ amqp.Table) (<-chan amqp.Delivery, error) {
	return c.deliveries, nil
}

// sagaRig is the real consumer and the real Holds service on an in-memory repository.
type sagaRig struct {
	svc      *app.Service
	channel  *fakeChannel
	acks     *ack
	deduper  *memoryDeduper
	confirmd *recordingEvents
}

type recordingEvents struct{ confirmed []domain.Hold }

func (r *recordingEvents) HoldRequested(context.Context, domain.Hold) error { return nil }
func (r *recordingEvents) HoldConfirmed(_ context.Context, h domain.Hold) error {
	r.confirmed = append(r.confirmed, h)
	return nil
}

func newSagaRig(t *testing.T) *sagaRig {
	t.Helper()
	events := &recordingEvents{}
	svc := app.NewService(memory.NewRepository(),
		app.Config{MaxActivePerUser: 3, HoldDuration: 2 * time.Hour},
		func() time.Time { return t0 },
		app.WithEvents(events),
		app.WithIDGenerator(func() (string, error) { return "h-1", nil }),
		app.WithCodeGenerator(func() (string, error) { return "ABC234", nil }))
	return &sagaRig{
		svc:      svc,
		channel:  &fakeChannel{deliveries: make(chan amqp.Delivery, 10)},
		acks:     &ack{},
		deduper:  newMemoryDeduper(),
		confirmd: events,
	}
}

// deliver feeds the events to the consumer and waits until each one has been settled.
func (r *sagaRig) deliver(t *testing.T, events ...amqp.Delivery) {
	t.Helper()
	consumer, err := NewStockConsumer(r.channel, r.deduper, r.svc, quietLog())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- consumer.Start(ctx) }()

	for i := range events {
		events[i].DeliveryTag = uint64(i + 1)
		events[i].Acknowledger = r.acks
		r.channel.deliveries <- events[i]
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		r.acks.mu.Lock()
		settled := len(r.acks.acked) + len(r.acks.nacks)
		r.acks.mu.Unlock()
		if settled == len(events) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d deliveries were settled", settled, len(events))
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("consumer stopped with %v", err)
	}
}

func (r *sagaRig) pendingHold(t *testing.T) {
	t.Helper()
	if _, err := r.svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 2); err != nil {
		t.Fatal(err)
	}
}

func message(d amqp.Delivery, id string) amqp.Delivery {
	d.MessageId = id
	return d
}

func TestTheConsumerListensForBothStockEvents(t *testing.T) {
	r := newSagaRig(t)
	r.deliver(t) // start and stop

	if r.channel.queue != StockQueue {
		t.Errorf("queue = %q, want %q", r.channel.queue, StockQueue)
	}
	if strings.Join(r.channel.bindings, ",") != "stock.reserved,stock.unavailable" {
		t.Errorf("bindings = %v, want stock.reserved and stock.unavailable", r.channel.bindings)
	}
}

func TestStockReservedEventConfirmsTheHoldEndToEnd(t *testing.T) {
	r := newSagaRig(t)
	r.pendingHold(t)

	r.deliver(t, message(delivery(t, "stock.reserved", stockReserved("h-1")), "event-1"))

	h, _ := r.svc.GetHold(context.Background(), "h-1")
	if h.Status != domain.Active || h.PickupCode != "ABC234" {
		t.Errorf("hold = %+v, want ACTIVE with the pickup code", h)
	}
	if len(r.confirmd.confirmed) != 1 {
		t.Errorf("hold.confirmed sent %d times, want 1", len(r.confirmd.confirmed))
	}
	if len(r.acks.acked) != 1 || len(r.acks.nacks) != 0 {
		t.Errorf("acked %v, nacked %v, want 1 ack", r.acks.acked, r.acks.nacks)
	}
}

func TestStockUnavailableEventRejectsTheHoldEndToEnd(t *testing.T) {
	r := newSagaRig(t)
	r.pendingHold(t)

	r.deliver(t, message(delivery(t, "stock.unavailable", stockUnavailable("h-1")), "event-1"))

	h, _ := r.svc.GetHold(context.Background(), "h-1")
	if h.Status != domain.Rejected {
		t.Errorf("hold is %s, want REJECTED", h.Status)
	}
	if len(r.confirmd.confirmed) != 0 {
		t.Error("hold.confirmed was sent for a rejected hold")
	}
}

// RabbitMQ delivers at least once: the same event (same message ID) can arrive twice.
func TestTheSameEventDeliveredTwiceIsHandledOnce(t *testing.T) {
	r := newSagaRig(t)
	r.pendingHold(t)
	event := message(delivery(t, "stock.reserved", stockReserved("h-1")), "event-1")

	r.deliver(t, event, event)

	if len(r.confirmd.confirmed) != 1 {
		t.Errorf("hold.confirmed sent %d times, want 1: the second delivery has the same event ID", len(r.confirmd.confirmed))
	}
	if len(r.acks.acked) != 2 || len(r.acks.nacks) != 0 {
		t.Errorf("acked %v, nacked %v, want both deliveries acknowledged", r.acks.acked, r.acks.nacks)
	}
}

// The same answer replayed under a new event ID gets past the event-ID check, so
// the saga itself must be safe: the hold must not change.
func TestTheSameAnswerUnderANewEventIDChangesNothing(t *testing.T) {
	r := newSagaRig(t)
	r.pendingHold(t)
	body := delivery(t, "stock.reserved", stockReserved("h-1"))

	r.deliver(t, message(body, "event-1"), message(body, "event-2"))

	h, _ := r.svc.GetHold(context.Background(), "h-1")
	if h.Status != domain.Active || h.PickupCode != "ABC234" || !h.ExpiresAt.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("hold = %+v, want it unchanged by the replay", h)
	}
	if len(r.confirmd.confirmed) != 2 || r.confirmd.confirmed[0] != r.confirmd.confirmed[1] {
		t.Errorf("hold.confirmed = %+v, want two identical sends", r.confirmd.confirmed)
	}
	if len(r.acks.acked) != 2 {
		t.Errorf("acked %v, want both acknowledged", r.acks.acked)
	}
}

func TestDuplicateStockUnavailableIsHandledOnce(t *testing.T) {
	r := newSagaRig(t)
	r.pendingHold(t)
	event := message(delivery(t, "stock.unavailable", stockUnavailable("h-1")), "event-1")

	r.deliver(t, event, event)

	h, _ := r.svc.GetHold(context.Background(), "h-1")
	if h.Status != domain.Rejected {
		t.Errorf("hold is %s, want REJECTED", h.Status)
	}
	if len(r.acks.acked) != 2 {
		t.Errorf("acked %v, want both acknowledged", r.acks.acked)
	}
}
