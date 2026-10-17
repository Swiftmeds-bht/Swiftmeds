package messaging

import (
	"context"
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

type memoryDeduper struct {
	processed map[string]bool
	checkErr  error
	markErr   error
}

func (d *memoryDeduper) IsProcessed(_ context.Context, eventID string) (bool, error) {
	if d.checkErr != nil {
		return false, d.checkErr
	}
	return d.processed[eventID], nil
}

func (d *memoryDeduper) MarkProcessed(_ context.Context, eventID string) error {
	if d.markErr != nil {
		return d.markErr
	}
	if d.processed == nil {
		d.processed = make(map[string]bool)
	}
	d.processed[eventID] = true
	return nil
}

type memoryAcknowledger struct {
	ackCount  int
	nackCount int
	requeue   []bool
}

func (a *memoryAcknowledger) Ack(_ uint64, _ bool) error {
	a.ackCount++
	return nil
}

func (a *memoryAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	a.nackCount++
	a.requeue = append(a.requeue, requeue)
	return nil
}

func (a *memoryAcknowledger) Reject(_ uint64, _ bool) error {
	return nil
}

func TestConsumerAcknowledgesAndMarksSuccessfulEvent(t *testing.T) {
	deduper := &memoryDeduper{}
	acknowledger := &memoryAcknowledger{}
	handlerCalls := 0

	consumer := &Consumer{
		config:  ConsumerConfig{MaxAttempts: 2},
		deduper: deduper,
		handler: func(context.Context, amqp.Delivery) error {
			handlerCalls++
			return nil
		},
	}
	delivery := amqp.Delivery{
		Acknowledger: acknowledger,
		MessageId:    "event-1",
		Body:         []byte("payload"),
	}

	if err := consumer.handleDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("handleDelivery() error = %v", err)
	}
	if handlerCalls != 1 {
		t.Errorf("handler calls = %d, want 1", handlerCalls)
	}
	if acknowledger.ackCount != 1 || acknowledger.nackCount != 0 {
		t.Errorf("acks = %d, nacks = %d; want 1 ack and 0 nacks",
			acknowledger.ackCount, acknowledger.nackCount)
	}
	if !deduper.processed["event-1"] {
		t.Error("successful event was not marked processed")
	}
}

func TestConsumerAcknowledgesDuplicateWithoutCallingHandler(t *testing.T) {
	deduper := &memoryDeduper{processed: map[string]bool{"event-1": true}}
	acknowledger := &memoryAcknowledger{}
	handlerCalls := 0

	consumer := &Consumer{
		config:  ConsumerConfig{MaxAttempts: 2},
		deduper: deduper,
		handler: func(context.Context, amqp.Delivery) error {
			handlerCalls++
			return nil
		},
	}
	delivery := amqp.Delivery{
		Acknowledger: acknowledger,
		MessageId:    "event-1",
	}

	if err := consumer.handleDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("handleDelivery() error = %v", err)
	}
	if handlerCalls != 0 {
		t.Errorf("handler calls = %d, want 0", handlerCalls)
	}
	if acknowledger.ackCount != 1 {
		t.Errorf("acks = %d, want 1", acknowledger.ackCount)
	}
}

func TestConsumerRetriesThenDeadLettersFailedEvent(t *testing.T) {
	deduper := &memoryDeduper{}
	acknowledger := &memoryAcknowledger{}
	handlerCalls := 0

	consumer := &Consumer{
		config:  ConsumerConfig{MaxAttempts: 2},
		deduper: deduper,
		handler: func(context.Context, amqp.Delivery) error {
			handlerCalls++
			return errors.New("temporary handler failure")
		},
	}
	delivery := amqp.Delivery{
		Acknowledger: acknowledger,
		MessageId:    "event-1",
	}

	if err := consumer.handleDelivery(context.Background(), delivery); err != nil {
		t.Fatalf("handleDelivery() error = %v", err)
	}
	if handlerCalls != 2 {
		t.Errorf("handler calls = %d, want 2", handlerCalls)
	}
	if acknowledger.nackCount != 1 || acknowledger.requeue[0] {
		t.Errorf("expected one Nack with requeue=false; got nacks=%d requeue=%v",
			acknowledger.nackCount, acknowledger.requeue)
	}
	if deduper.processed["event-1"] {
		t.Error("failed event was marked processed")
	}
}

func TestConsumerRequeuesWhenDeduperCheckFails(t *testing.T) {
	deduper := &memoryDeduper{checkErr: errors.New("database unavailable")}
	acknowledger := &memoryAcknowledger{}

	consumer := &Consumer{
		config:  ConsumerConfig{MaxAttempts: 2},
		deduper: deduper,
		handler: func(context.Context, amqp.Delivery) error {
			t.Error("handler called after deduper check failed")
			return nil
		},
	}
	delivery := amqp.Delivery{
		Acknowledger: acknowledger,
		MessageId:    "event-1",
	}

	if err := consumer.handleDelivery(context.Background(), delivery); err == nil {
		t.Fatal("handleDelivery() error = nil, want deduper error")
	}
	if acknowledger.nackCount != 1 || !acknowledger.requeue[0] {
		t.Errorf("expected one Nack with requeue=true; got nacks=%d requeue=%v",
			acknowledger.nackCount, acknowledger.requeue)
	}
}
