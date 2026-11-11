//go:build integration

package saga

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
)

// eventLog records every event on the swiftmed.events exchange, so a test can
// check what the services sent and replay a message to test duplicates.
type eventLog struct {
	conn *amqp.Connection
	ch   *amqp.Channel
	mu   sync.Mutex
	seen []amqp.Delivery
}

func watchEvents(amqpURL string) (*eventLog, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	// Same declaration as pkg/messaging, so whoever comes first creates it.
	if err := ch.ExchangeDeclare(messaging.ExchangeName, "topic", true, false, false, false, nil); err != nil {
		_ = conn.Close()
		return nil, err
	}
	q, err := ch.QueueDeclare("", false, true, true, false, nil) // private, deleted with the connection
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := ch.QueueBind(q.Name, "#", messaging.ExchangeName, false, nil); err != nil {
		_ = conn.Close()
		return nil, err
	}
	deliveries, err := ch.Consume(q.Name, "", true, true, false, false, nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	l := &eventLog{conn: conn, ch: ch}
	go func() {
		for d := range deliveries {
			l.mu.Lock()
			l.seen = append(l.seen, d)
			l.mu.Unlock()
		}
	}()
	return l, nil
}

func (l *eventLog) Close() { _ = l.conn.Close() }

// holdIDOf reads hold_id from any saga event (they all carry it as field 2).
func holdIDOf(d amqp.Delivery) string {
	var e interface{ GetHoldId() string }
	switch d.RoutingKey {
	case "hold.requested":
		e = &eventsv1.HoldRequested{}
	case "stock.reserved":
		e = &eventsv1.StockReserved{}
	case "stock.unavailable":
		e = &eventsv1.StockUnavailable{}
	case "hold.confirmed":
		e = &eventsv1.HoldConfirmed{}
	default:
		return ""
	}
	if err := proto.Unmarshal(d.Body, e.(proto.Message)); err != nil {
		return ""
	}
	return e.GetHoldId()
}

// For returns the events with this routing key about this hold, in order.
func (l *eventLog) For(routingKey, holdID string) []amqp.Delivery {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []amqp.Delivery
	for _, d := range l.seen {
		if d.RoutingKey == routingKey && holdIDOf(d) == holdID {
			out = append(out, d)
		}
	}
	return out
}

// WaitFor waits until at least one such event arrived and returns the first.
func (l *eventLog) WaitFor(t *testing.T, routingKey, holdID string) amqp.Delivery {
	t.Helper()
	var got []amqp.Delivery
	eventually(t, 15*time.Second, fmt.Sprintf("%s for hold %s", routingKey, holdID), func() bool {
		got = l.For(routingKey, holdID)
		return len(got) > 0
	})
	return got[0]
}

// Replay publishes d again exactly as it was sent: same routing key, body,
// content type and message ID (the event_id that consumers deduplicate on).
func (l *eventLog) Replay(t *testing.T, d amqp.Delivery) {
	t.Helper()
	l.replay(t, d, d.MessageId)
}

// ReplayAs publishes d again under a new message ID, as a producer retry would.
func (l *eventLog) ReplayAs(t *testing.T, d amqp.Delivery, messageID string) {
	t.Helper()
	l.replay(t, d, messageID)
}

func (l *eventLog) replay(t *testing.T, d amqp.Delivery, messageID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := l.ch.PublishWithContext(ctx, messaging.ExchangeName, d.RoutingKey, false, false, amqp.Publishing{
		ContentType:   d.ContentType,
		DeliveryMode:  amqp.Persistent,
		MessageId:     messageID,
		CorrelationId: d.CorrelationId,
		Body:          d.Body,
	})
	if err != nil {
		t.Fatalf("replay %s: %v", d.RoutingKey, err)
	}
}

// eventually polls cond until it is true or the timeout passes.
func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", timeout, what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// consistently checks cond stays true for the whole period, for "nothing else happens".
func consistently(t *testing.T, period time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(period)
	for time.Now().Before(deadline) {
		if !cond() {
			t.Fatalf("%s stopped holding", what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
