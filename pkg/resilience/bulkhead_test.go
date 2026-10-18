package resilience

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBulkheadRejectsCallsWhenFull(t *testing.T) {
	b := NewBulkhead(2)
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	var wg sync.WaitGroup

	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Execute(func() error {
				started <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	<-started
	<-started

	called := false
	err := b.Execute(func() error { called = true; return nil })
	if !errors.Is(err, ErrBulkheadFull) {
		t.Fatalf("err = %v, want ErrBulkheadFull", err)
	}
	if called {
		t.Error("call ran while the bulkhead was full")
	}

	close(release)
	wg.Wait()
}

func TestBulkheadFreesTheSlotAfterEachCall(t *testing.T) {
	b := NewBulkhead(1)

	if err := b.Execute(func() error { return errBoom }); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
	// The failed call must still have given its slot back.
	if err := b.Execute(ok); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestBulkheadNeverRunsMoreCallsThanTheLimit(t *testing.T) {
	const limit = 5
	b := NewBulkhead(limit)
	var running, highest, rejected atomic.Int32
	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := b.Execute(func() error {
				n := running.Add(1)
				for {
					h := highest.Load()
					if n <= h || highest.CompareAndSwap(h, n) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond)
				running.Add(-1)
				return nil
			})
			if errors.Is(err, ErrBulkheadFull) {
				rejected.Add(1)
			}
		}()
	}
	wg.Wait()

	if highest.Load() > limit {
		t.Fatalf("highest concurrent calls = %d, want at most %d", highest.Load(), limit)
	}
	t.Logf("highest = %d, rejected = %d", highest.Load(), rejected.Load())
}

func TestNewBulkheadWithZeroStillAllowsOneCall(t *testing.T) {
	if err := NewBulkhead(0).Execute(ok); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}
