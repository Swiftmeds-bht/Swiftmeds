package grpc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// switchableInventory fails while down is true, like Inventory being unreachable.
type switchableInventory struct{ down bool }

func (s *switchableInventory) CheckStocked(context.Context, string, string) error { return s.result() }
func (s *switchableInventory) Release(context.Context, string) error              { return s.result() }
func (s *switchableInventory) Commit(context.Context, string) error               { return s.result() }
func (s *switchableInventory) result() error {
	if s.down {
		return domain.ErrInventoryUnavailable
	}
	return nil
}

type switchableEvents struct{ down bool }

func (s *switchableEvents) HoldRequested(context.Context, domain.Hold) error { return s.result() }
func (s *switchableEvents) HoldConfirmed(context.Context, domain.Hold) error { return s.result() }
func (s *switchableEvents) HoldExpired(context.Context, domain.Hold) error   { return s.result() }
func (s *switchableEvents) HoldRejected(context.Context, domain.Hold) error  { return s.result() }
func (s *switchableEvents) result() error {
	if s.down {
		return errors.New("rabbitmq is down")
	}
	return nil
}

// newSagaFixture is a handler whose Inventory and event publisher can be switched off.
func newSagaFixture(t *testing.T) (*fixture, *switchableInventory, *switchableEvents) {
	t.Helper()
	inv, ev := &switchableInventory{}, &switchableEvents{}
	f := &fixture{now: t0}
	n := 0
	cfg := app.Config{MaxActivePerUser: 3, HoldDuration: 2 * time.Hour}
	f.svc = app.NewService(memory.NewRepository(), cfg, func() time.Time { return f.now },
		app.WithIDGenerator(func() (string, error) { n++; return fmt.Sprintf("h-%d", n), nil }),
		app.WithCodeGenerator(func() (string, error) { return testCode, nil }),
		app.WithInventory(inv), app.WithEvents(ev))
	f.h = NewHandler(f.svc)
	return f, inv, ev
}

func TestCreateHoldIsUnavailableWhenTheEventCannotBeSent(t *testing.T) {
	f, _, ev := newSagaFixture(t)
	ev.down = true

	_, err := f.h.CreateHold(context.Background(), &holdsv1.CreateHoldRequest{
		UserId: "u-1", PharmacyId: "ph-1", ProductId: "med-1", Quantity: 1,
	})

	wantCode(t, err, codes.Unavailable)
	// The patient is told to try again and gets no internal detail.
	if got := status.Convert(err).Message(); got != "stock service is not available, please try again" {
		t.Errorf("message = %q", got)
	}
}

func TestCancelIsUnavailableWhenInventoryIsDownAndTheHoldStaysActive(t *testing.T) {
	f, inv, _ := newSagaFixture(t)
	hold := f.activeHold(t)
	inv.down = true

	_, err := f.h.CancelHold(context.Background(), &holdsv1.CancelHoldRequest{HoldId: hold.GetHoldId(), UserId: "u-1"})

	wantCode(t, err, codes.Unavailable)
	got, _ := f.h.GetHold(context.Background(), &holdsv1.GetHoldRequest{HoldId: hold.GetHoldId()})
	if got.GetHold().GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_ACTIVE {
		t.Errorf("hold is %s, want it still ACTIVE", got.GetHold().GetStatus())
	}

	inv.down = false
	if _, err := f.h.CancelHold(context.Background(), &holdsv1.CancelHoldRequest{HoldId: hold.GetHoldId(), UserId: "u-1"}); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestCollectIsUnavailableWhenInventoryIsDownAndWorksOnRetry(t *testing.T) {
	f, inv, _ := newSagaFixture(t)
	hold := f.activeHold(t)
	inv.down = true
	req := &holdsv1.CollectHoldRequest{HoldId: hold.GetHoldId(), PharmacyId: "ph-1", PickupCode: testCode}

	_, err := f.h.CollectHold(context.Background(), req)
	wantCode(t, err, codes.Unavailable)

	inv.down = false
	res, err := f.h.CollectHold(context.Background(), req)
	if err != nil || res.GetHold().GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_COLLECTED {
		t.Fatalf("retry = (%v, %v), want a COLLECTED hold", res, err)
	}
}

func TestExpireIsUnavailableWhenInventoryIsDown(t *testing.T) {
	f, inv, _ := newSagaFixture(t)
	hold := f.activeHold(t)
	f.now = t0.Add(3 * time.Hour)
	inv.down = true

	_, err := f.h.ExpireHold(context.Background(), &holdsv1.ExpireHoldRequest{HoldId: hold.GetHoldId()})

	wantCode(t, err, codes.Unavailable)
}
