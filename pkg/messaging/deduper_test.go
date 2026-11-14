package messaging

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryDeduperRejectsEmptyID(t *testing.T) {
	if _, _, err := NewMemoryDeduper().Claim(context.Background(), ""); !errors.Is(err, ErrEmptyEventID) {
		t.Fatalf("Claim(\"\") error = %v, want ErrEmptyEventID", err)
	}
}

func TestMemoryDeduperCommitMakesLaterClaimsDuplicates(t *testing.T) {
	ctx := context.Background()
	d := NewMemoryDeduper()

	claim, claimed, err := d.Claim(ctx, "event-1")
	if err != nil || !claimed {
		t.Fatalf("first Claim() = %v, %v; want claimed", claimed, err)
	}
	if err := claim.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// A second Commit or Release is a no-op.
	if err := claim.Release(ctx); err != nil {
		t.Fatal(err)
	}

	if _, claimed, err := d.Claim(ctx, "event-1"); err != nil || claimed {
		t.Fatalf("Claim() after Commit = %v, %v; want a duplicate", claimed, err)
	}
	if _, claimed, _ := d.Claim(ctx, "event-2"); !claimed {
		t.Fatal("another event ID was not claimable")
	}
}

func TestMemoryDeduperReleaseFreesTheID(t *testing.T) {
	ctx := context.Background()
	d := NewMemoryDeduper()

	claim, _, _ := d.Claim(ctx, "event-1")
	if err := claim.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if d.IsProcessed("event-1") {
		t.Fatal("released event is marked processed")
	}
	if _, claimed, err := d.Claim(ctx, "event-1"); err != nil || !claimed {
		t.Fatalf("Claim() after Release = %v, %v; want claimed", claimed, err)
	}
}

func TestMemoryDeduperWaitingClaimHonoursContext(t *testing.T) {
	d := NewMemoryDeduper()
	if _, claimed, _ := d.Claim(context.Background(), "event-1"); !claimed {
		t.Fatal("first claim failed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, _, err := d.Claim(ctx, "event-1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Claim() while held = %v, want DeadlineExceeded", err)
	}
}

// Many goroutines deliver the same event ID at once, and every holder of a
// claim randomly commits or releases it. At most one Commit may ever happen,
// and once it has, nobody else gets a claim. Run with -race.
func TestMemoryDeduperClaimIsAtomicUnderContention(t *testing.T) {
	const goroutines = 200
	ctx := context.Background()
	d := NewMemoryDeduper()

	var holders, committed atomic.Int32
	var concurrent, maxConcurrent atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claim, claimed, err := d.Claim(ctx, "event-1")
			if err != nil {
				t.Errorf("Claim() error = %v", err)
				return
			}
			if !claimed {
				return
			}
			holders.Add(1)
			now := concurrent.Add(1)
			for {
				prev := maxConcurrent.Load()
				if now <= prev || maxConcurrent.CompareAndSwap(prev, now) {
					break
				}
			}
			concurrent.Add(-1)
			// Release the first few claims to prove a released ID is handed to exactly one waiter.
			if i%4 != 0 || holders.Load() < 5 {
				_ = claim.Release(ctx)
				return
			}
			committed.Add(1)
			_ = claim.Commit(ctx)
		}()
	}
	close(start)
	wg.Wait()

	if got := maxConcurrent.Load(); got != 1 {
		t.Errorf("%d claims were held at the same time, want 1", got)
	}
	if got := committed.Load(); got > 1 {
		t.Errorf("event committed %d times, want at most 1", got)
	}
	if committed.Load() == 1 && !d.IsProcessed("event-1") {
		t.Error("committed event is not marked processed")
	}
}
