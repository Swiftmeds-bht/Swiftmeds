package app

import (
	"context"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

var (
	_ ports.Inventory = (*fakeInventory)(nil)
	_ ports.Events    = (*fakeEvents)(nil)
)

type inventoryCall struct{ op, holdID string }

// fakeInventory records every Release and Commit. Set err to make calls fail.
// onCall runs at the start of each call, so a test can look at the hold at that moment.
type fakeInventory struct {
	calls  []inventoryCall
	err    error
	onCall func(op, holdID string)
}

func (f *fakeInventory) record(op, holdID string) error {
	f.calls = append(f.calls, inventoryCall{op, holdID})
	if f.onCall != nil {
		f.onCall(op, holdID)
	}
	return f.err
}

func (f *fakeInventory) Release(_ context.Context, holdID string) error {
	return f.record("release", holdID)
}

func (f *fakeInventory) Commit(_ context.Context, holdID string) error {
	return f.record("commit", holdID)
}

// count returns how many calls of that kind were made.
func (f *fakeInventory) count(op string) int {
	n := 0
	for _, c := range f.calls {
		if c.op == op {
			n++
		}
	}
	return n
}

// fakeEvents records the events that would be published. Set errRequested or
// errConfirmed to make that kind of publish fail.
type fakeEvents struct {
	requested    []domain.Hold
	confirmed    []domain.Hold
	errRequested error
	errConfirmed error
}

func (f *fakeEvents) HoldRequested(_ context.Context, h domain.Hold) error {
	f.requested = append(f.requested, h)
	return f.errRequested
}

func (f *fakeEvents) HoldConfirmed(_ context.Context, h domain.Hold) error {
	f.confirmed = append(f.confirmed, h)
	return f.errConfirmed
}

// sagaService is a Service on an in-memory repository with a fake Inventory and fake events.
func sagaService(t *testing.T) (*Service, *fakeInventory, *fakeEvents, *clock) {
	t.Helper()
	inv, ev := &fakeInventory{}, &fakeEvents{}
	svc, _, clk := newTestService(t, WithInventory(inv), WithEvents(ev))
	return svc, inv, ev, clk
}

// pendingHold creates a PENDING hold for u-1 at ph-1 (and clears the recorded event).
func pendingHold(t *testing.T, svc *Service, ev *fakeEvents) domain.Hold {
	t.Helper()
	h, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	ev.requested = nil
	return h
}
