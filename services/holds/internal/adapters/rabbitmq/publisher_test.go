package rabbitmq

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

var t0 = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

var errBoom = errors.New("boom")

type sent struct {
	key   string
	event proto.Message
}

// fakePublisher records what would go to RabbitMQ.
type fakePublisher struct {
	sent []sent
	err  error
}

func (f *fakePublisher) Publish(_ context.Context, key string, event proto.Message) error {
	f.sent = append(f.sent, sent{key, event})
	return f.err
}

func pendingHold(t *testing.T) domain.Hold {
	t.Helper()
	h, err := domain.NewHold("h-1", "u-1", "ph-1", "med-1", 2, t0)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func activeHold(t *testing.T) domain.Hold {
	t.Helper()
	h, err := pendingHold(t).Reserve(t0.Add(time.Minute), 2*time.Hour, "ABC234")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestHoldRequestedCarriesTheHoldAndStaysDeterministic(t *testing.T) {
	fake := &fakePublisher{}
	p, _ := NewPublisher(fake)
	h := pendingHold(t)

	if err := p.HoldRequested(context.Background(), h); err != nil {
		t.Fatal(err)
	}

	if len(fake.sent) != 1 || fake.sent[0].key != "hold.requested" {
		t.Fatalf("sent %v, want one event with routing key hold.requested", fake.sent)
	}
	got, ok := fake.sent[0].event.(*eventsv1.HoldRequested)
	if !ok {
		t.Fatalf("event is %T, want *HoldRequested", fake.sent[0].event)
	}
	if got.GetHoldId() != "h-1" || got.GetUserId() != "u-1" || got.GetPharmacyId() != "ph-1" ||
		got.GetProductId() != "med-1" || got.GetQuantity() != 2 {
		t.Errorf("event = %v, want the hold's details", got)
	}
	if got.GetExpiresAt() != nil {
		t.Error("a PENDING hold has no expiry time yet, so the event must not carry one")
	}
	m := got.GetMetadata()
	if m.GetCorrelationId() != "h-1" || m.GetProducer() != "holds" || !m.GetOccurredAt().AsTime().Equal(t0) {
		t.Errorf("metadata = %v, want correlation h-1, producer holds, time %v", m, t0)
	}
	if m.GetEventId() != EventID("hold.requested", "h-1") {
		t.Errorf("event ID = %q, want the deterministic one", m.GetEventId())
	}
}

func TestHoldConfirmedCarriesThePickupCodeAndExpiry(t *testing.T) {
	fake := &fakePublisher{}
	p, _ := NewPublisher(fake)
	h := activeHold(t)

	if err := p.HoldConfirmed(context.Background(), h); err != nil {
		t.Fatal(err)
	}

	if len(fake.sent) != 1 || fake.sent[0].key != "hold.confirmed" {
		t.Fatalf("sent %v, want one event with routing key hold.confirmed", fake.sent)
	}
	got := fake.sent[0].event.(*eventsv1.HoldConfirmed)
	if got.GetPickupCode() != "ABC234" || !got.GetExpiresAt().AsTime().Equal(h.ExpiresAt) {
		t.Errorf("event = %v, want pickup code ABC234 expiring at %v", got, h.ExpiresAt)
	}
	if got.GetHoldId() != "h-1" || got.GetUserId() != "u-1" || got.GetPharmacyId() != "ph-1" ||
		got.GetProductId() != "med-1" || got.GetQuantity() != 2 {
		t.Errorf("event = %v, want the hold's details", got)
	}
	if !got.GetMetadata().GetOccurredAt().AsTime().Equal(h.StatusChangedAt) {
		t.Errorf("occurred_at = %v, want the time the hold became ACTIVE (%v)", got.GetMetadata().GetOccurredAt().AsTime(), h.StatusChangedAt)
	}
}

// Sending hold.confirmed a second time must produce exactly the same message,
// so that a listener sees the same event ID and ignores it.
func TestSendingHoldConfirmedAgainGivesIdenticalBytes(t *testing.T) {
	fake := &fakePublisher{}
	p, _ := NewPublisher(fake)
	h := activeHold(t)

	_ = p.HoldConfirmed(context.Background(), h)
	_ = p.HoldConfirmed(context.Background(), h)

	first, _ := proto.MarshalOptions{Deterministic: true}.Marshal(fake.sent[0].event)
	second, _ := proto.MarshalOptions{Deterministic: true}.Marshal(fake.sent[1].event)
	if string(first) != string(second) {
		t.Error("the two hold.confirmed messages differ")
	}
}

func TestOnlyAnActiveHoldCanBeConfirmed(t *testing.T) {
	fake := &fakePublisher{}
	p, _ := NewPublisher(fake)

	err := p.HoldConfirmed(context.Background(), pendingHold(t))

	if err == nil {
		t.Fatal("err = nil, want an error for a PENDING hold")
	}
	if len(fake.sent) != 0 {
		t.Error("an event was published for a hold that is not ACTIVE")
	}
}

func TestPublishErrorsAreReturned(t *testing.T) {
	p, _ := NewPublisher(&fakePublisher{err: errBoom})

	if err := p.HoldRequested(context.Background(), pendingHold(t)); !errors.Is(err, errBoom) {
		t.Errorf("HoldRequested: err = %v, want the publish error", err)
	}
	if err := p.HoldConfirmed(context.Background(), activeHold(t)); !errors.Is(err, errBoom) {
		t.Errorf("HoldConfirmed: err = %v, want the publish error", err)
	}
}

func TestNewPublisherNeedsAPublisher(t *testing.T) {
	if _, err := NewPublisher(nil); err == nil {
		t.Error("err = nil, want an error")
	}
}

func TestEventIDIsStableUniqueAndUUIDShaped(t *testing.T) {
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	a := EventID("hold.confirmed", "h-1")

	if !uuid.MatchString(a) {
		t.Errorf("EventID = %q, want a version 5 UUID", a)
	}
	if a != EventID("hold.confirmed", "h-1") {
		t.Error("the same kind and hold must give the same ID")
	}
	for name, other := range map[string]string{
		"another hold": EventID("hold.confirmed", "h-2"),
		"another kind": EventID("hold.requested", "h-1"),
	} {
		if other == a {
			t.Errorf("%s gave the same ID", name)
		}
	}
}
