package messaging

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRabbitMQIntegration_PublishConsumed(t *testing.T) {
	brokerURL := os.Getenv("RABBITMQ_URL")
	if brokerURL == "" {
		t.Skip("set RABBITMQ_URL to run the RabbitMQ integration test")
	}

	connection, err := amqp.Dial(brokerURL)
	if err != nil {
		t.Fatalf("connect to RabbitMQ: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	publishChannel, err := connection.Channel()
	if err != nil {
		t.Fatalf("open publisher channel: %v", err)
	}
	t.Cleanup(func() { _ = publishChannel.Close() })

	consumeChannel, err := connection.Channel()
	if err != nil {
		t.Fatalf("open consumer channel: %v", err)
	}
	t.Cleanup(func() { _ = consumeChannel.Close() })

	queueName := fmt.Sprintf("messaging.integration.%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = consumeChannel.QueueDelete(queueName, false, false, false)
		_, _ = consumeChannel.QueueDelete(queueName+".dlq", false, false, false)
	})

	handled := make(chan struct{}, 1)
	consumer, err := NewConsumer(
		consumeChannel,
		ConsumerConfig{
			Queue:       queueName,
			RoutingKeys: []string{"hold.requested"},
			MaxAttempts: 3,
		},
		&memoryDeduper{},
		func(context.Context, amqp.Delivery) error {
			handled <- struct{}{}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumerDone := make(chan error, 1)
	go func() {
		consumerDone <- consumer.Start(ctx)
	}()

	publisher, err := NewPublisher(publishChannel)
	if err != nil {
		t.Fatalf("create publisher: %v", err)
	}

	event := &eventsv1.HoldRequested{
		Metadata: &eventsv1.EventMetadata{
			EventId: fmt.Sprintf("integration-%d", time.Now().UnixNano()),
		},
	}
	if err := publisher.Publish(ctx, "hold.requested", event); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	select {
	case <-handled:
	case <-time.After(10 * time.Second):
		t.Fatal("consumer did not receive the published event")
	}

	cancel()
	select {
	case err := <-consumerDone:
		if err != nil {
			t.Fatalf("consumer stopped with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not stop after cancellation")
	}
}
