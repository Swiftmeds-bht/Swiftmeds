//go:build integration

package messaging

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Run with a throwaway database:
//
//	MESSAGING_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/messaging_test?sslmode=disable \
//	  go test -tags integration -race ./messaging/
func openDedupDB(t *testing.T) *PostgresDeduper {
	t.Helper()
	url := os.Getenv("MESSAGING_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set MESSAGING_TEST_DATABASE_URL to run the Postgres deduper integration test")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	d, err := NewPostgresDeduper(db)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func uniqueEventID(t *testing.T) string {
	return fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
}

func TestPostgresDeduperIntegration_CommitAndRelease(t *testing.T) {
	d := openDedupDB(t)
	ctx := context.Background()

	if _, _, err := d.Claim(ctx, ""); !errors.Is(err, ErrEmptyEventID) {
		t.Fatalf("Claim(\"\") error = %v, want ErrEmptyEventID", err)
	}

	released := uniqueEventID(t) + "-released"
	claim, claimed, err := d.Claim(ctx, released)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %v, %v; want claimed", claimed, err)
	}
	if err := claim.Release(ctx); err != nil {
		t.Fatal(err)
	}
	claim, claimed, err = d.Claim(ctx, released)
	if err != nil || !claimed {
		t.Fatalf("Claim() after Release = %v, %v; want claimed again", claimed, err)
	}
	if err := claim.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := claim.Release(ctx); err != nil {
		t.Fatalf("Release() after Commit = %v, want a no-op", err)
	}
	if _, claimed, err := d.Claim(ctx, released); err != nil || claimed {
		t.Fatalf("Claim() after Commit = %v, %v; want a duplicate", claimed, err)
	}
}

func TestPostgresDeduperIntegration_WaitingClaimGetsIDAfterRollback(t *testing.T) {
	d := openDedupDB(t)
	ctx := context.Background()
	id := uniqueEventID(t)

	first, claimed, err := d.Claim(ctx, id)
	if err != nil || !claimed {
		t.Fatalf("first Claim() = %v, %v", claimed, err)
	}

	type result struct {
		claim   Claim
		claimed bool
		err     error
	}
	second := make(chan result, 1)
	go func() {
		c, ok, err := d.Claim(ctx, id)
		second <- result{c, ok, err}
	}()

	select {
	case r := <-second:
		t.Fatalf("second Claim() returned %v, %v while the first was open; want it to wait", r.claimed, r.err)
	case <-time.After(200 * time.Millisecond):
	}

	if err := first.Release(ctx); err != nil {
		t.Fatal(err)
	}
	r := <-second
	if r.err != nil || !r.claimed {
		t.Fatalf("second Claim() after Release = %v, %v; want claimed", r.claimed, r.err)
	}
	if err := r.claim.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresDeduperIntegration_WaitingClaimHonoursContext(t *testing.T) {
	d := openDedupDB(t)
	id := uniqueEventID(t)

	first, _, err := d.Claim(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Release(context.Background()) })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, _, err := d.Claim(ctx, id); err == nil {
		t.Fatal("Claim() while held = nil error, want the context error")
	}
}

// The SCRUM-132 race: many consumers receive the same message at once. With
// the old read-then-write check, several could run the handler.
func TestPostgresDeduperIntegration_ConcurrentConsumersRunOneHandler(t *testing.T) {
	const consumers = 20
	d := openDedupDB(t)
	id := uniqueEventID(t)
	acknowledger := &memoryAcknowledger{}
	var handlerCalls atomic.Int32

	handler := func(context.Context, amqp.Delivery) error {
		handlerCalls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	start := make(chan struct{})
	errs := make(chan error, consumers)
	var wg sync.WaitGroup
	for range consumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			consumer := &Consumer{config: ConsumerConfig{MaxAttempts: 1}, deduper: d, handler: handler}
			<-start
			errs <- consumer.handleDelivery(context.Background(),
				amqp.Delivery{Acknowledger: acknowledger, MessageId: id})
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
	if acknowledger.ackCount != consumers {
		t.Errorf("acks = %d, want %d", acknowledger.ackCount, consumers)
	}
}

// A failed handler must not lose the message: the claim is rolled back and a
// redelivery is handled.
func TestPostgresDeduperIntegration_FailedHandlerLeavesEventClaimable(t *testing.T) {
	d := openDedupDB(t)
	id := uniqueEventID(t)
	acknowledger := &memoryAcknowledger{}
	fail := true
	calls := 0

	consumer := &Consumer{
		config:  ConsumerConfig{MaxAttempts: 1},
		deduper: d,
		handler: func(context.Context, amqp.Delivery) error {
			calls++
			if fail {
				return errors.New("handler failed")
			}
			return nil
		},
	}
	delivery := amqp.Delivery{Acknowledger: acknowledger, MessageId: id}

	if err := consumer.handleDelivery(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	fail = false
	if err := consumer.handleDelivery(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	if err := consumer.handleDelivery(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("handler ran %d times, want 2 (failed, redelivered; the third copy is a duplicate)", calls)
	}
}
