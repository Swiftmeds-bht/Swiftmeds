package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/resilience"
)

const (
	// DialTimeout is how long Dial keeps trying to reach RabbitMQ. RabbitMQ
	// can take half a minute to open its port on a busy machine (27.6 s on a CI
	// runner), and the services that use it often start first.
	DialTimeout = 60 * time.Second

	// attemptTimeout bounds one try: the TCP connect plus the AMQP handshake.
	attemptTimeout = 10 * time.Second

	// The wait after a failed try starts at dialBaseDelay and doubles up to
	// dialMaxDelay, with jitter, so services started together do not retry
	// in step.
	dialBaseDelay = 250 * time.Millisecond
	dialMaxDelay  = 5 * time.Second
)

// Dial connects to RabbitMQ at url (amqp:// or amqps://), retrying while the
// broker is not reachable yet, for up to DialTimeout or until ctx ends.
//
// Errors a retry cannot fix return at once: a malformed URL, wrong
// credentials, or no access to the vhost. Each failed try is logged at warn
// level, without the URL, because it holds the password.
func Dial(ctx context.Context, url string, log *slog.Logger) (*amqp.Connection, error) {
	return dial(ctx, url, log, dialOptions{})
}

// dialOptions are what tests replace. Zero fields use the real ones.
type dialOptions struct {
	dial    func(url string) (*amqp.Connection, error)
	timeout time.Duration
	sleep   func(ctx context.Context, d time.Duration) error
	jitter  func(d time.Duration) time.Duration
}

func dial(ctx context.Context, url string, log *slog.Logger, opts dialOptions) (*amqp.Connection, error) {
	if _, err := amqp.ParseURI(url); err != nil {
		return nil, fmt.Errorf("RabbitMQ URL: %w", err)
	}
	if opts.dial == nil {
		opts.dial = dialOnce
	}
	if opts.timeout <= 0 {
		opts.timeout = DialTimeout
	}
	if log == nil {
		log = slog.Default()
	}

	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	start := time.Now()
	attempts := 0
	var conn *amqp.Connection
	err := resilience.Retry(ctx, resilience.RetryConfig{
		MaxAttempts: math.MaxInt, // the time budget (ctx) is the limit
		BaseDelay:   dialBaseDelay,
		MaxDelay:    dialMaxDelay,
		Retryable:   retryableDialError,
		Sleep:       opts.sleep,
		Jitter:      opts.jitter,
	}, func(context.Context) error {
		attempts++
		c, err := opts.dial(url)
		if err != nil {
			if retryableDialError(err) {
				log.Warn("RabbitMQ not reachable yet, retrying",
					slog.Int("attempt", attempts), slog.String("error", err.Error()))
			}
			return err
		}
		conn = c
		return nil
	})
	if err != nil {
		if !retryableDialError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("RabbitMQ not reachable after %d tries in %s: %w",
			attempts, time.Since(start).Round(time.Second), err)
	}
	if attempts > 1 {
		log.Info("connected to RabbitMQ",
			// As text: the JSON handler would write a Duration as nanoseconds.
			slog.Int("attempts", attempts), slog.String("waited", time.Since(start).Round(time.Millisecond).String()))
	}
	return conn, nil
}

// dialOnce is amqp.Dial with a shorter limit on one try (amqp.Dial allows 30 s).
func dialOnce(url string) (*amqp.Connection, error) {
	return amqp.DialConfig(url, amqp.Config{
		Locale: "en_US", // what amqp.Dial sets
		Dial:   amqp.DefaultDial(attemptTimeout),
	})
}

// retryableDialError reports whether waiting could help. RabbitMQ creates its
// users before it opens the AMQP port, so "access refused" is final.
func retryableDialError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var amqpErr *amqp.Error
	if errors.As(err, &amqpErr) && amqpErr.Code == amqp.AccessRefused {
		return false
	}
	return true
}
