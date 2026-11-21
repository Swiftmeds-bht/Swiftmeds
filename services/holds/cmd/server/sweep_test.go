package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/app"
)

// countingSweeper answers each sweep with the next result, and counts the calls.
type countingSweeper struct {
	mu      sync.Mutex
	calls   int
	results []app.SweepResult
	errs    []error
}

func (s *countingSweeper) SweepStaleHolds(context.Context) (app.SweepResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.calls
	s.calls++
	var res app.SweepResult
	var err error
	if i < len(s.results) {
		res = s.results[i]
	}
	if i < len(s.errs) {
		err = s.errs[i]
	}
	return res, err
}

func (s *countingSweeper) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// syncBuffer is a log destination safe to read while the sweep loop writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestSweepPeriodicallyKeepsGoingAfterAFailureAndStopsOnCancel(t *testing.T) {
	s := &countingSweeper{
		results: []app.SweepResult{{Rejected: 1, ReleaseFailed: 1}, {Released: 1}, {}},
		errs:    []error{errBoom},
	}
	var logs syncBuffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sweepPeriodically(ctx, s, time.Millisecond, log); close(done) }()

	deadline := time.Now().Add(5 * time.Second)
	for s.count() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("sweep loop did not stop after cancel")
	}

	if s.count() < 3 {
		t.Fatalf("sweeps = %d, want at least 3 (a failed sweep must not stop the loop)", s.count())
	}
	out := logs.String()
	for _, want := range []string{"stale hold sweep had failures", "release_failed=1", "stale holds recovered", "released=1"} {
		if !strings.Contains(out, want) {
			t.Errorf("log is missing %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "stale holds recovered") != 1 {
		t.Errorf("an empty sweep must not log:\n%s", out)
	}
}
