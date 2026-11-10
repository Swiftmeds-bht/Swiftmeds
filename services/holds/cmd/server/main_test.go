package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

var errBoom = errors.New("boom")

// start returns a context, the function that cancels it, and a channel the test fills
// with what the "gRPC server" and the "event consumer" return.
func start() (context.Context, context.CancelFunc, chan error) {
	ctx, cancel := context.WithCancel(context.Background())
	return ctx, cancel, make(chan error, 2)
}

func TestWaitForRuntimeStopsTheOtherPartWhenOneFails(t *testing.T) {
	ctx, cancel, results := start()
	stopped := false
	stop := func() { stopped = true; cancel() }
	results <- errBoom          // the first part fails
	results <- context.Canceled // the other part stops once it is cancelled

	err := waitForRuntime(ctx, stop, results)

	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the first failure", err)
	}
	if !stopped {
		t.Error("the other part was not stopped")
	}
}

func TestWaitForRuntimeReportsTheSecondFailureWhenTheFirstEndedCleanly(t *testing.T) {
	ctx, cancel, results := start()
	results <- nil
	results <- errBoom

	err := waitForRuntime(ctx, cancel, results)

	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the second failure", err)
	}
}

// If either part stops, even without an error, the service must not keep running half alive.
func TestWaitForRuntimeTreatsAnUnexpectedStopAsAnError(t *testing.T) {
	ctx, cancel, results := start()
	results <- nil
	results <- nil

	err := waitForRuntime(ctx, cancel, results)

	if err == nil || !strings.Contains(err.Error(), "stopped unexpectedly") {
		t.Fatalf("err = %v, want a stopped-unexpectedly error", err)
	}
}

func TestWaitForRuntimeIsQuietOnAnOrdinaryShutdown(t *testing.T) {
	ctx, cancel, results := start()
	cancel() // Ctrl+C or docker stop
	results <- nil
	results <- context.Canceled

	if err := waitForRuntime(ctx, cancel, results); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestWaitForRuntimeReportsAFailureDuringShutdown(t *testing.T) {
	for name, order := range map[string][2]error{
		"first":  {errBoom, nil},
		"second": {nil, errBoom},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel, results := start()
			cancel()
			results <- order[0]
			results <- order[1]

			err := waitForRuntime(ctx, cancel, results)

			if !errors.Is(err, errBoom) {
				t.Fatalf("err = %v, want the shutdown failure", err)
			}
		})
	}
}
