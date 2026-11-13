package messaging

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// scriptedDeduper fails on demand and counts how claims end.
type scriptedDeduper struct {
	claimErr  error
	commitErr error
	commits   int
	releases  int
}

func (d *scriptedDeduper) Claim(context.Context, string) (Claim, bool, error) {
	if d.claimErr != nil {
		return nil, false, d.claimErr
	}
	return scriptedClaim{d}, true, nil
}

type scriptedClaim struct{ d *scriptedDeduper }

func (c scriptedClaim) Commit(context.Context) error {
	c.d.commits++
	return c.d.commitErr
}

func (c scriptedClaim) Release(context.Context) error {
	c.d.releases++
	return nil
}

type memoryAcknowledger struct {
	mu        sync.Mutex
	ackCount  int
	nackCount int
	requeue   []bool
}

func (a *memoryAcknowledger) Ack(_ uint64, _ bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ackCount++
	return nil
}

func (a *memoryAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.nackCount++
	a.requeue = append(a.requeue, requeue)
	return nil
}

func (a *memoryAcknowledger) Reject(_ uint64, _ bool) error {
	return nil
}

func TestConsumerAcknowledgesAndMarksSuccessfulEvent(t *testing.T) {
	deduper := NewMemoryDeduper()
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
	if !deduper.IsProcessed("event-1") {
		t.Error("successful event was not marked processed")
	}
}

func TestConsumerAcknowledgesDuplicateWithoutCallingHandler(t *testing.T) {
	deduper := NewMemoryDeduper()
	claim, _, _ := deduper.Claim(context.Background(), "event-1")
	_ = claim.Commit(context.Background())
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
	deduper := NewMemoryDeduper()
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
	if deduper.IsProcessed("event-1") {
		t.Error("failed event was marked processed")
	}

	// The claim was released, so a replay from the dead-letter queue is handled.
	claim, claimed, err := deduper.Claim(context.Background(), "event-1")
	if err != nil || !claimed {
		t.Fatalf("Claim() after dead-lettering = %v, %v; want the ID to be free", claimed, err)
	}
	_ = claim.Release(context.Background())
}

func TestConsumerRequeuesWhenDeduperClaimFails(t *testing.T) {
	deduper := &scriptedDeduper{claimErr: errors.New("database unavailable")}
	acknowledger := &memoryAcknowledger{}

	consumer := &Consumer{
		config:  ConsumerConfig{MaxAttempts: 2},
		deduper: deduper,
		handler: func(context.Context, amqp.Delivery) error {
			t.Error("handler called after deduper claim failed")
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

func TestConsumerRequeuesWhenCommitFails(t *testing.T) {
	deduper := &scriptedDeduper{commitErr: errors.New("commit failed")}
	acknowledger := &memoryAcknowledger{}

	consumer := &Consumer{
		config:  ConsumerConfig{MaxAttempts: 2},
		deduper: deduper,
		handler: func(context.Context, amqp.Delivery) error { return nil },
	}
	delivery := amqp.Delivery{Acknowledger: acknowledger, MessageId: "event-1"}

	if err := consumer.handleDelivery(context.Background(), delivery); err == nil {
		t.Fatal("handleDelivery() error = nil, want the commit error")
	}
	if acknowledger.ackCount != 0 || acknowledger.nackCount != 1 || !acknowledger.requeue[0] {
		t.Errorf("acks = %d, nacks = %d, requeue = %v; want one requeue and no ack",
			acknowledger.ackCount, acknowledger.nackCount, acknowledger.requeue)
	}
}

func TestConsumerReleasesClaimWhenStoppedDuringRetryDelay(t *testing.T) {
	deduper := &scriptedDeduper{}
	acknowledger := &memoryAcknowledger{}
	ctx, cancel := context.WithCancel(context.Background())

	consumer := &Consumer{
		config:  ConsumerConfig{MaxAttempts: 3, RetryDelay: time.Hour},
		deduper: deduper,
		handler: func(context.Context, amqp.Delivery) error {
			cancel()
			return errors.New("temporary handler failure")
		},
	}
	delivery := amqp.Delivery{Acknowledger: acknowledger, MessageId: "event-1"}

	if err := consumer.handleDelivery(ctx, delivery); !errors.Is(err, context.Canceled) {
		t.Fatalf("handleDelivery() error = %v, want context.Canceled", err)
	}
	if deduper.releases != 1 || deduper.commits != 0 {
		t.Errorf("releases = %d, commits = %d; want the claim released", deduper.releases, deduper.commits)
	}
	if acknowledger.nackCount != 1 || !acknowledger.requeue[0] {
		t.Errorf("nacks = %d, requeue = %v; want one requeue", acknowledger.nackCount, acknowledger.requeue)
	}
}

// Many consumers receive the same message at the same time (a redelivery while
// the first copy is still being handled). Exactly one handler may run.
func TestConsumerRunsOneHandlerForConcurrentDuplicates(t *testing.T) {
	const consumers = 64
	deduper := NewMemoryDeduper()
	acknowledger := &memoryAcknowledger{}
	var handlerCalls atomic.Int32

	handler := func(context.Context, amqp.Delivery) error {
		handlerCalls.Add(1)
		time.Sleep(5 * time.Millisecond) // keep the claim open while the others arrive
		return nil
	}

	start := make(chan struct{})
	errs := make(chan error, consumers)
	var wg sync.WaitGroup
	for range consumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumer := &Consumer{config: ConsumerConfig{MaxAttempts: 1}, deduper: deduper, handler: handler}
			<-start
			errs <- consumer.handleDelivery(context.Background(),
				amqp.Delivery{Acknowledger: acknowledger, MessageId: "event-1"})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("handleDelivery() error = %v", err)
		}
	}
	if got := handlerCalls.Load(); got != 1 {
		t.Errorf("handler ran %d times, want 1", got)
	}
	if acknowledger.ackCount != consumers || acknowledger.nackCount != 0 {
		t.Errorf("acks = %d, nacks = %d; want %d acks", acknowledger.ackCount, acknowledger.nackCount, consumers)
	}
}

// If the consumer that holds the claim fails, the message is not lost: a
// concurrent copy that was waiting for the claim handles it instead.
func TestConsumerWaitingDuplicateHandlesEventWhenFirstHandlerFails(t *testing.T) {
	deduper := NewMemoryDeduper()
	acknowledger := &memoryAcknowledger{}
	var calls atomic.Int32
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	handler := func(context.Context, amqp.Delivery) error {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
			return errors.New("first copy fails")
		}
		return nil
	}
	newConsumer := func() *Consumer {
		return &Consumer{config: ConsumerConfig{MaxAttempts: 1}, deduper: deduper, handler: handler}
	}
	delivery := amqp.Delivery{Acknowledger: acknowledger, MessageId: "event-1"}

	firstDone := make(chan error, 1)
	go func() { firstDone <- newConsumer().handleDelivery(context.Background(), delivery) }()
	<-firstStarted

	secondDone := make(chan error, 1)
	go func() { secondDone <- newConsumer().handleDelivery(context.Background(), delivery) }()

	select {
	case err := <-secondDone:
		close(releaseFirst)
		t.Fatalf("second copy finished while the first still held the claim (err: %v)", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first handleDelivery() error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second handleDelivery() error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("handler ran %d times, want 2 (failed first copy, then the waiting copy)", got)
	}
	if !deduper.IsProcessed("event-1") {
		t.Error("event was not marked processed after the second copy succeeded")
	}
}
