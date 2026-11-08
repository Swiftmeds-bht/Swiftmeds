package rabbitmq

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
)

var t0 = time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type publication struct {
	exchange string
	key      string
	msg      amqp.Publishing
}

type fakeChannel struct {
	declareErr error
	publishErr error
	declared   []string
	published  []publication
}

func (c *fakeChannel) ExchangeDeclare(name, _ string, _, _, _, _ bool, _ amqp.Table) error {
	c.declared = append(c.declared, name)
	return c.declareErr
}

func (c *fakeChannel) PublishWithContext(_ context.Context, exchange, key string, _, _ bool, msg amqp.Publishing) error {
	if c.publishErr != nil {
		return c.publishErr
	}
	c.published = append(c.published, publication{exchange, key, msg})
	return nil
}

func feedback() domain.Feedback {
	return domain.Feedback{
		ID: "f-1", HoldID: "h-1", PharmacyID: "ph-1", ProductID: "med-1",
		WasInStock: false, Rating: 2, Comment: "private words that must not leave the service", CreatedAt: t0,
	}
}

func newPublisher(t *testing.T, ch *fakeChannel) *Publisher {
	t.Helper()
	p, err := NewPublisher(ch)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func decode(t *testing.T, pub publication) *eventsv1.FeedbackSubmitted {
	t.Helper()
	var event eventsv1.FeedbackSubmitted
	if err := proto.Unmarshal(pub.msg.Body, &event); err != nil {
		t.Fatalf("body is not a FeedbackSubmitted: %v", err)
	}
	return &event
}

// "Event published with event_id."
func TestPublishFeedbackSubmittedSendsTheEventWithAnEventID(t *testing.T) {
	ch := &fakeChannel{}
	p := newPublisher(t, ch)

	if err := p.PublishFeedbackSubmitted(context.Background(), feedback()); err != nil {
		t.Fatal(err)
	}

	if len(ch.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(ch.published))
	}
	pub := ch.published[0]
	if pub.exchange != messaging.ExchangeName || pub.key != "feedback.submitted" {
		t.Errorf("sent to %q with key %q, want %q and feedback.submitted", pub.exchange, pub.key, messaging.ExchangeName)
	}
	if pub.msg.DeliveryMode != amqp.Persistent || pub.msg.ContentType != messaging.ProtobufType {
		t.Errorf("delivery mode %d, content type %q, want persistent protobuf", pub.msg.DeliveryMode, pub.msg.ContentType)
	}

	event := decode(t, pub)
	eventID := event.GetMetadata().GetEventId()
	if !uuidPattern.MatchString(eventID) {
		t.Errorf("event_id = %q, want a version 4 UUID", eventID)
	}
	if pub.msg.MessageId != eventID {
		t.Errorf("message id = %q, want the event_id %q", pub.msg.MessageId, eventID)
	}
	if event.GetMetadata().GetProducer() != "feedback" || event.GetMetadata().GetCorrelationId() != "h-1" ||
		!event.GetMetadata().GetOccurredAt().AsTime().Equal(t0) {
		t.Errorf("metadata = %v, want producer feedback, correlation h-1, occurred_at %v", event.GetMetadata(), t0)
	}
	if event.GetFeedbackId() != "f-1" || event.GetHoldId() != "h-1" || event.GetPharmacyId() != "ph-1" ||
		event.GetProductId() != "med-1" || event.GetWasInStock() || event.GetRating() != 2 {
		t.Errorf("event = %v", event)
	}
}

func TestEveryEventGetsItsOwnEventID(t *testing.T) {
	ch := &fakeChannel{}
	p := newPublisher(t, ch)

	for range 2 {
		if err := p.PublishFeedbackSubmitted(context.Background(), feedback()); err != nil {
			t.Fatal(err)
		}
	}

	first := decode(t, ch.published[0]).GetMetadata().GetEventId()
	second := decode(t, ch.published[1]).GetMetadata().GetEventId()
	if first == second {
		t.Errorf("both events have event_id %q, want different ones", first)
	}
}

// Minimal personal data (risk R8): the event has no field for the patient, and
// the comment stays in the Feedback service.
func TestTheEventCarriesNoPatientIDAndNoComment(t *testing.T) {
	fields := (&eventsv1.FeedbackSubmitted{}).ProtoReflect().Descriptor().Fields()
	for _, name := range []protoreflect.Name{"user_id", "comment"} {
		if fields.ByName(name) != nil {
			t.Errorf("FeedbackSubmitted has a %q field, want none", name)
		}
	}
}

func TestPublishFailureIsReturned(t *testing.T) {
	boom := errors.New("connection closed")
	p := newPublisher(t, &fakeChannel{publishErr: boom})

	err := p.PublishFeedbackSubmitted(context.Background(), feedback())

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the channel's error", err)
	}
}

func TestNewPublisherDeclaresTheExchangeAndRejectsABadChannel(t *testing.T) {
	ch := &fakeChannel{}
	newPublisher(t, ch)
	if len(ch.declared) != 1 || ch.declared[0] != messaging.ExchangeName {
		t.Errorf("declared %v, want [%s]", ch.declared, messaging.ExchangeName)
	}

	if _, err := NewPublisher(nil); err == nil {
		t.Error("NewPublisher(nil) succeeded, want an error")
	}
	if _, err := NewPublisher(&fakeChannel{declareErr: errors.New("no permission")}); err == nil {
		t.Error("NewPublisher with a failing ExchangeDeclare succeeded, want an error")
	}
}

func TestANilPublisherFailsInsteadOfPanicking(t *testing.T) {
	var p *Publisher
	if err := p.PublishFeedbackSubmitted(context.Background(), feedback()); err == nil {
		t.Error("nil publisher published, want an error")
	}
}
