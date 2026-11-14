package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const DeadLetterExchange = "swiftmed.events.dlx"

type consumerChannel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	Qos(prefetchCount, prefetchSize int, global bool) error
	ConsumeWithContext(
		ctx context.Context,
		queue, consumer string,
		autoAck, exclusive, noLocal, noWait bool,
		args amqp.Table,
	) (<-chan amqp.Delivery, error)
}

type ConsumerConfig struct {
	Queue       string
	RoutingKeys []string
	Prefetch    int
	MaxAttempts int
	RetryDelay  time.Duration
}

// EventHandler processes one RabbitMQ delivery.
type EventHandler func(context.Context, amqp.Delivery) error

// Consumer receives events and acknowledges or dead-letters each delivery.
type Consumer struct {
	channel consumerChannel
	config  ConsumerConfig
	deduper Deduper
	handler EventHandler
}

// NewConsumer declares the event queue and its dead-letter queue.
func NewConsumer(
	channel consumerChannel,
	config ConsumerConfig,
	deduper Deduper,
	handler EventHandler,
) (*Consumer, error) {
	if channel == nil {
		return nil, errors.New("RabbitMQ channel is nil")
	}
	if strings.TrimSpace(config.Queue) == "" {
		return nil, errors.New("consumer queue is empty")
	}
	if len(config.RoutingKeys) == 0 {
		return nil, errors.New("consumer has no routing keys")
	}
	for _, key := range config.RoutingKeys {
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("consumer has an empty routing key")
		}
	}
	if deduper == nil {
		return nil, errors.New("deduper is nil")
	}
	if handler == nil {
		return nil, errors.New("event handler is nil")
	}
	if config.Prefetch <= 0 {
		config.Prefetch = 1
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.RetryDelay < 0 {
		return nil, errors.New("retry delay cannot be negative")
	}

	if err := channel.ExchangeDeclare(
		ExchangeName, "topic", true, false, false, false, nil,
	); err != nil {
		return nil, fmt.Errorf("declare event exchange: %w", err)
	}
	if err := channel.ExchangeDeclare(
		DeadLetterExchange, "direct", true, false, false, false, nil,
	); err != nil {
		return nil, fmt.Errorf("declare dead-letter exchange: %w", err)
	}

	deadLetterQueue, err := channel.QueueDeclare(
		config.Queue+".dlq", true, false, false, false, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("declare dead-letter queue: %w", err)
	}
	if err := channel.QueueBind(
		deadLetterQueue.Name, config.Queue, DeadLetterExchange, false, nil,
	); err != nil {
		return nil, fmt.Errorf("bind dead-letter queue: %w", err)
	}

	queueArgs := amqp.Table{
		"x-dead-letter-exchange":    DeadLetterExchange,
		"x-dead-letter-routing-key": config.Queue,
	}
	if _, err := channel.QueueDeclare(
		config.Queue, true, false, false, false, queueArgs,
	); err != nil {
		return nil, fmt.Errorf("declare event queue: %w", err)
	}

	for _, key := range config.RoutingKeys {
		if err := channel.QueueBind(config.Queue, key, ExchangeName, false, nil); err != nil {
			return nil, fmt.Errorf("bind routing key %q: %w", key, err)
		}
	}

	return &Consumer{
		channel: channel,
		config:  config,
		deduper: deduper,
		handler: handler,
	}, nil
}

// Start consumes deliveries until the context is cancelled or RabbitMQ reports an error.
func (c *Consumer) Start(ctx context.Context) error {
	if err := c.channel.Qos(c.config.Prefetch, 0, false); err != nil {
		return fmt.Errorf("set consumer prefetch: %w", err)
	}

	deliveries, err := c.channel.ConsumeWithContext(
		ctx, c.config.Queue, "", false, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("start consuming queue %q: %w", c.config.Queue, err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("RabbitMQ delivery channel closed")
			}
			if err := c.handleDelivery(ctx, delivery); err != nil {
				return err
			}
		}
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) error {
	if delivery.MessageId == "" {
		slog.WarnContext(ctx, "dead-lettering event without message ID")
		return delivery.Nack(false, false)
	}

	// Claim checks and reserves the event ID in one step, so two consumers that
	// receive the same message cannot both run the handler (SCRUM-132).
	claim, claimed, err := c.deduper.Claim(ctx, delivery.MessageId)
	if err != nil {
		nackErr := delivery.Nack(false, true)
		return errors.Join(
			fmt.Errorf("claim event %q for deduplication: %w", delivery.MessageId, err),
			wrapNackError(nackErr),
		)
	}
	if !claimed {
		return delivery.Ack(false)
	}

	var handlerErr error
	for attempt := 1; attempt <= c.config.MaxAttempts; attempt++ {
		handlerErr = c.handler(ctx, delivery)
		if handlerErr == nil {
			break
		}
		if attempt < c.config.MaxAttempts && c.config.RetryDelay > 0 {
			timer := time.NewTimer(c.config.RetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				// Shutting down: give the ID back so the requeued delivery is handled.
				releaseErr := releaseClaim(ctx, claim, delivery.MessageId)
				nackErr := delivery.Nack(false, true)
				return errors.Join(ctx.Err(), releaseErr, wrapNackError(nackErr))
			case <-timer.C:
			}
		}
	}

	if handlerErr != nil {
		// The event was not handled, so it is not recorded: if it is replayed
		// from the dead-letter queue, it is handled then.
		releaseErr := releaseClaim(ctx, claim, delivery.MessageId)
		if err := delivery.Nack(false, false); err != nil {
			return errors.Join(fmt.Errorf("dead-letter event %q: %w", delivery.MessageId, err), releaseErr)
		}
		slog.ErrorContext(ctx, "event handler failed; delivery dead-lettered",
			"message_id", delivery.MessageId,
			"attempts", c.config.MaxAttempts,
			"error", handlerErr,
		)
		if releaseErr != nil {
			slog.ErrorContext(ctx, "release event claim", "message_id", delivery.MessageId, "error", releaseErr)
		}
		return nil
	}

	if err := claim.Commit(context.WithoutCancel(ctx)); err != nil {
		// The handler ran but the record was not saved. Requeue: the event is
		// handled again (at least once), which handlers must tolerate anyway.
		nackErr := delivery.Nack(false, true)
		return errors.Join(err, wrapNackError(nackErr))
	}
	return delivery.Ack(false)
}

// releaseClaim gives a claim back. It must run even when ctx is already cancelled.
func releaseClaim(ctx context.Context, claim Claim, eventID string) error {
	if err := claim.Release(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("release event %q: %w", eventID, err)
	}
	return nil
}

func wrapNackError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("nack delivery: %w", err)
}
