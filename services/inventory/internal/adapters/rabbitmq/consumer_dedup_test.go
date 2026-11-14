package rabbitmq

import (
	"context"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
)

type inMemoryRabbitChannel struct {
	deliveries    <-chan amqp.Delivery
	consumerQueue string
	bindings      []string
}

func (*inMemoryRabbitChannel) ExchangeDeclare(
	_, _ string, _, _, _, _ bool, _ amqp.Table,
) error {
	return nil
}

func (*inMemoryRabbitChannel) QueueDeclare(
	name string, _, _, _, _ bool, _ amqp.Table,
) (amqp.Queue, error) {
	return amqp.Queue{Name: name}, nil
}

func (c *inMemoryRabbitChannel) QueueBind(
	queue, key, _ string, _ bool, _ amqp.Table,
) error {
	c.bindings = append(c.bindings, queue+":"+key)
	return nil
}

func (*inMemoryRabbitChannel) Qos(_, _ int, _ bool) error {
	return nil
}

func (c *inMemoryRabbitChannel) ConsumeWithContext(
	_ context.Context,
	queue, _ string,
	_, _, _, _ bool,
	_ amqp.Table,
) (<-chan amqp.Delivery, error) {
	c.consumerQueue = queue
	return c.deliveries, nil
}

type inMemoryAcknowledger struct {
	acked  []uint64
	nacked []uint64
	cancel context.CancelFunc
}

func (a *inMemoryAcknowledger) Ack(tag uint64, _ bool) error {
	a.acked = append(a.acked, tag)
	if len(a.acked) == 2 {
		a.cancel()
	}
	return nil
}

func (a *inMemoryAcknowledger) Nack(tag uint64, _, _ bool) error {
	a.nacked = append(a.nacked, tag)
	return nil
}

func (*inMemoryAcknowledger) Reject(uint64, bool) error {
	return nil
}

func TestDuplicateHoldRequestedDeliveryReservesOnce(t *testing.T) {
	stock, err := domain.NewStock(
		"pharmacy-1",
		"product-1",
		10,
		0,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("create stock: %v", err)
	}

	reserveCalls := 0
	repo := &consumerRepo{
		reserveFn: func(
			_ context.Context,
			_ string,
			_ ports.StockKey,
			_ int32,
		) (domain.Stock, bool, error) {
			reserveCalls++
			return stock, true, nil
		},
	}
	service := consumerTestService(t, repo)

	body, err := proto.Marshal(&eventsv1.HoldRequested{
		HoldId:     "hold-1",
		PharmacyId: "pharmacy-1",
		ProductId:  "product-1",
		Quantity:   2,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	acker := &inMemoryAcknowledger{cancel: cancel}
	deliveries := make(chan amqp.Delivery, 2)
	for tag := uint64(1); tag <= 2; tag++ {
		deliveries <- amqp.Delivery{
			Acknowledger: acker,
			DeliveryTag:  tag,
			MessageId:    "event-1",
			ContentType:  messaging.ProtobufType,
			Body:         body,
		}
	}

	broker := &inMemoryRabbitChannel{deliveries: deliveries}
	deduper := messaging.NewMemoryDeduper()

	consumer, err := NewHoldRequestedConsumer(broker, deduper, service)
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}
	if err := consumer.Start(ctx); err != nil {
		t.Fatalf("start consumer: %v", err)
	}

	if reserveCalls != 1 {
		t.Errorf("Reserve called %d times; want 1", reserveCalls)
	}
	if len(acker.acked) != 2 {
		t.Errorf("acknowledged %d deliveries; want 2", len(acker.acked))
	}
	if len(acker.nacked) != 0 {
		t.Errorf("nacked %d deliveries; want 0", len(acker.nacked))
	}
	if !deduper.IsProcessed("event-1") {
		t.Error("event ID was not marked processed")
	}
	if broker.consumerQueue != "inventory.hold-requested" {
		t.Errorf("consumer queue = %q", broker.consumerQueue)
	}

	wantBinding := "inventory.hold-requested:hold.requested"
	foundBinding := false
	for _, binding := range broker.bindings {
		if binding == wantBinding {
			foundBinding = true
			break
		}
	}
	if !foundBinding {
		t.Errorf("missing binding %q; got %v", wantBinding, broker.bindings)
	}
}
