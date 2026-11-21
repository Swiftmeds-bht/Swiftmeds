package messaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const testURL = "amqp://swiftmed:s3cret-pass@rabbitmq:5672/"

var errRefused = fmt.Errorf("dial tcp 10.0.0.5:5672: %w", syscall.ECONNREFUSED)

// scriptedDial fails with the given errors in order, then connects.
type scriptedDial struct {
	errs  []error
	calls int
	conn  *amqp.Connection
}

func (s *scriptedDial) dial(string) (*amqp.Connection, error) {
	s.calls++
	if s.calls <= len(s.errs) {
		return nil, s.errs[s.calls-1]
	}
	return s.conn, nil
}

func testLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil)), &buf
}

// noWait records the waits instead of sleeping.
type noWait struct{ waits []time.Duration }

func (n *noWait) sleep(ctx context.Context, d time.Duration) error {
	n.waits = append(n.waits, d)
	return ctx.Err()
}

func identity(d time.Duration) time.Duration { return d }

func TestDialRetriesUntilTheBrokerAccepts(t *testing.T) {
	want := &amqp.Connection{}
	script := &scriptedDial{errs: []error{errRefused, errRefused, errRefused}, conn: want}
	waits := &noWait{}
	log, logs := testLogger()

	got, err := dial(context.Background(), testURL, log, dialOptions{
		dial: script.dial, sleep: waits.sleep, jitter: identity,
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if got != want {
		t.Fatal("dial did not return the connection")
	}
	if script.calls != 4 {
		t.Fatalf("tries = %d, want 4", script.calls)
	}
	wantWaits := []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second}
	if fmt.Sprint(waits.waits) != fmt.Sprint(wantWaits) {
		t.Fatalf("waits = %v, want %v", waits.waits, wantWaits)
	}
	if n := strings.Count(logs.String(), "RabbitMQ not reachable yet, retrying"); n != 3 {
		t.Fatalf("retry log lines = %d, want 3:\n%s", n, logs)
	}
	if !strings.Contains(logs.String(), "connected to RabbitMQ") {
		t.Fatalf("no success line after retries:\n%s", logs)
	}
}

func TestDialWaitIsCappedAtFiveSeconds(t *testing.T) {
	script := &scriptedDial{errs: repeat(errRefused, 8), conn: &amqp.Connection{}}
	waits := &noWait{}
	log, _ := testLogger()

	if _, err := dial(context.Background(), testURL, log, dialOptions{
		dial: script.dial, sleep: waits.sleep, jitter: identity,
	}); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if last := waits.waits[len(waits.waits)-1]; last != 5*time.Second {
		t.Fatalf("last wait = %v, want the 5s cap (waits %v)", last, waits.waits)
	}
}

func TestDialConnectsFirstTimeWithoutLogging(t *testing.T) {
	script := &scriptedDial{conn: &amqp.Connection{}}
	log, logs := testLogger()

	if _, err := dial(context.Background(), testURL, log, dialOptions{dial: script.dial}); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if script.calls != 1 || logs.Len() != 0 {
		t.Fatalf("tries = %d, logs = %q; want 1 try and no logs", script.calls, logs)
	}
}

func TestDialGivesUpWhenTheTimeBudgetEnds(t *testing.T) {
	script := &scriptedDial{errs: repeat(errRefused, 1000)}
	log, _ := testLogger()

	start := time.Now()
	_, err := dial(context.Background(), testURL, log, dialOptions{
		dial: script.dial, timeout: 50 * time.Millisecond, jitter: func(time.Duration) time.Duration { return time.Millisecond },
	})
	if err == nil {
		t.Fatal("dial succeeded, want an error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %v, want about the 50ms budget", elapsed)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("error %v does not wrap the last dial error", err)
	}
	if !strings.Contains(err.Error(), "RabbitMQ not reachable after") || script.calls < 2 {
		t.Fatalf("error = %v after %d tries, want a give-up error after several", err, script.calls)
	}
}

func TestDialDoesNotRetryWhatWaitingCannotFix(t *testing.T) {
	for name, dialErr := range map[string]error{
		"wrong credentials": amqp.ErrCredentials,
		"no vhost access":   amqp.ErrVhost,
	} {
		t.Run(name, func(t *testing.T) {
			script := &scriptedDial{errs: []error{dialErr}}
			waits := &noWait{}
			log, logs := testLogger()

			_, err := dial(context.Background(), testURL, log, dialOptions{dial: script.dial, sleep: waits.sleep})
			if !errors.Is(err, dialErr) {
				t.Fatalf("error = %v, want %v", err, dialErr)
			}
			if script.calls != 1 || len(waits.waits) != 0 {
				t.Fatalf("tries = %d, waits = %v; want one try and no wait", script.calls, waits.waits)
			}
			if logs.Len() != 0 {
				t.Fatalf("logged a retry for a final error: %s", logs)
			}
		})
	}
}

func TestDialRejectsAMalformedURLWithoutDialling(t *testing.T) {
	script := &scriptedDial{}
	log, _ := testLogger()

	_, err := dial(context.Background(), "http://rabbitmq:5672", log, dialOptions{dial: script.dial})
	if err == nil || !strings.Contains(err.Error(), "RabbitMQ URL") {
		t.Fatalf("error = %v, want a URL error", err)
	}
	if script.calls != 0 {
		t.Fatalf("dialled %d times for a malformed URL", script.calls)
	}
}

func TestDialStopsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	script := &scriptedDial{errs: repeat(errRefused, 100)}
	log, _ := testLogger()

	_, err := dial(ctx, testURL, log, dialOptions{
		dial: func(url string) (*amqp.Connection, error) {
			cancel() // shutting down during the first try
			return script.dial(url)
		},
	})
	if err == nil {
		t.Fatal("dial succeeded, want an error")
	}
	if script.calls != 1 {
		t.Fatalf("tries = %d after cancel, want 1", script.calls)
	}
}

func TestDialNeverLogsThePassword(t *testing.T) {
	script := &scriptedDial{errs: []error{errRefused, errRefused}, conn: &amqp.Connection{}}
	log, logs := testLogger()

	if _, err := dial(context.Background(), testURL, log, dialOptions{
		dial: script.dial, sleep: (&noWait{}).sleep,
	}); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if strings.Contains(logs.String(), "s3cret-pass") {
		t.Fatalf("logs contain the password:\n%s", logs)
	}
}

// With the real dialler: a port nobody listens on is retried, then given up.
func TestDialRetriesARealClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listens there now

	log, logs := testLogger()
	_, err = dial(context.Background(), "amqp://guest:guest@"+addr+"/", log, dialOptions{
		timeout: 400 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("dial succeeded against a closed port")
	}
	if !retryableDialError(errors.Unwrap(err)) || !strings.Contains(err.Error(), "not reachable after") {
		t.Fatalf("error = %v, want a give-up error", err)
	}
	if n := strings.Count(logs.String(), "retrying"); n < 2 {
		t.Fatalf("retried %d times in 400ms, want at least 2:\n%s", n, logs)
	}
}

func repeat(err error, n int) []error {
	errs := make([]error, n)
	for i := range errs {
		errs[i] = err
	}
	return errs
}
