//go:build integration

package saga

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
)

// Scenarios from SCRUM-56: happy path, stock unavailable -> REJECTED, hold
// expires -> stock released, and a duplicate event delivered twice. Cancel is
// included too, because "stock is never left reserved after compensation"
// covers both compensation paths (expire and cancel).

func TestHappyPathReservesStockAndConfirmsTheHold(t *testing.T) {
	dumpLogsOnFailure(t)
	s := newScenario(t)
	s.setStock(5)

	hold := s.createHold(2)
	if hold.GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_PENDING {
		t.Fatalf("new hold is %s, want PENDING until Inventory answers", hold.GetStatus())
	}

	active := s.waitStatus(hold.GetHoldId(), holdsv1.HoldStatus_HOLD_STATUS_ACTIVE)
	if len(active.GetPickupCode()) == 0 {
		t.Error("active hold has no pickup code")
	}
	if !active.GetExpiresAt().AsTime().After(active.GetStatusChangedAt().AsTime()) {
		t.Errorf("expires_at %v is not after activation %v", active.GetExpiresAt().AsTime(), active.GetStatusChangedAt().AsTime())
	}
	s.assertStock(5, 2, 3)

	// The whole chain went over RabbitMQ.
	stack.events.WaitFor(t, "hold.requested", hold.GetHoldId())
	stack.events.WaitFor(t, "stock.reserved", hold.GetHoldId())
	stack.events.WaitFor(t, "hold.confirmed", hold.GetHoldId())
}

func TestStockUnavailableRejectsTheHoldAndReservesNothing(t *testing.T) {
	dumpLogsOnFailure(t)
	s := newScenario(t)
	s.setStock(1)

	hold := s.createHold(3)
	rejected := s.waitStatus(hold.GetHoldId(), holdsv1.HoldStatus_HOLD_STATUS_REJECTED)
	if !strings.Contains(rejected.GetStatusReason(), "stock unavailable") {
		t.Errorf("status_reason = %q, want it to say stock unavailable", rejected.GetStatusReason())
	}
	if rejected.GetPickupCode() != "" {
		t.Error("a rejected hold has a pickup code")
	}

	stack.events.WaitFor(t, "stock.unavailable", hold.GetHoldId())
	consistently(t, 2*time.Second, "no reservation and no confirmation", func() bool {
		st := s.stock()
		return st.GetQuantityReserved() == 0 && st.GetQuantityAvailable() == 1 &&
			len(stack.events.For("hold.confirmed", hold.GetHoldId())) == 0
	})
}

func TestExpiredHoldReleasesItsStock(t *testing.T) {
	dumpLogsOnFailure(t)
	s := newScenario(t)
	s.setStock(4)

	hold := s.createHold(2)
	active := s.waitStatus(hold.GetHoldId(), holdsv1.HoldStatus_HOLD_STATUS_ACTIVE)
	s.assertStock(4, 2, 2)

	// Too early: the hold stays active and keeps its stock.
	_, err := stack.holds.ExpireHold(ctx(t), &holdsv1.ExpireHoldRequest{HoldId: hold.GetHoldId()})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("early ExpireHold: code %v, want FailedPrecondition", status.Code(err))
	}
	s.assertStock(4, 2, 2)

	// Once expires_at has passed, Expiry's call gives the units back.
	time.Sleep(time.Until(active.GetExpiresAt().AsTime()) + 200*time.Millisecond)
	resp, err := stack.holds.ExpireHold(ctx(t), &holdsv1.ExpireHoldRequest{HoldId: hold.GetHoldId()})
	if err != nil {
		t.Fatalf("ExpireHold: %v", err)
	}
	if got := resp.GetHold().GetStatus(); got != holdsv1.HoldStatus_HOLD_STATUS_EXPIRED {
		t.Fatalf("after ExpireHold the hold is %s, want EXPIRED", got)
	}
	s.assertStock(4, 0, 4)

	// Expiry may call again (it retries); nothing changes.
	if _, err := stack.holds.ExpireHold(ctx(t), &holdsv1.ExpireHoldRequest{HoldId: hold.GetHoldId()}); err != nil {
		t.Fatalf("second ExpireHold: %v", err)
	}
	s.assertStock(4, 0, 4)
}

func TestCancelledHoldReleasesItsStock(t *testing.T) {
	dumpLogsOnFailure(t)
	s := newScenario(t)
	s.setStock(3)

	hold := s.createHold(1)
	s.waitStatus(hold.GetHoldId(), holdsv1.HoldStatus_HOLD_STATUS_ACTIVE)
	s.assertStock(3, 1, 2)

	resp, err := stack.holds.CancelHold(ctx(t), &holdsv1.CancelHoldRequest{HoldId: hold.GetHoldId(), UserId: s.user, Reason: "changed my mind"})
	if err != nil {
		t.Fatalf("CancelHold: %v", err)
	}
	if got := resp.GetHold().GetStatus(); got != holdsv1.HoldStatus_HOLD_STATUS_CANCELLED {
		t.Fatalf("after CancelHold the hold is %s, want CANCELLED", got)
	}
	s.assertStock(3, 0, 3)
}

func TestDuplicateEventsChangeNothing(t *testing.T) {
	dumpLogsOnFailure(t)
	s := newScenario(t)
	s.setStock(5)

	hold := s.createHold(2)
	active := s.waitStatus(hold.GetHoldId(), holdsv1.HoldStatus_HOLD_STATUS_ACTIVE)
	s.assertStock(5, 2, 3)
	requested := stack.events.WaitFor(t, "hold.requested", hold.GetHoldId())
	reserved := stack.events.WaitFor(t, "stock.reserved", hold.GetHoldId())

	// The same messages delivered again (same event_id): pkg/messaging drops them.
	stack.events.Replay(t, requested)
	stack.events.Replay(t, reserved)
	// The same events again under new event_ids, as a producer retry would send:
	// the handlers themselves must be idempotent (one reservation per hold).
	stack.events.ReplayAs(t, requested, "retry-"+requested.MessageId)
	stack.events.ReplayAs(t, reserved, "retry-"+reserved.MessageId)

	consistently(t, 3*time.Second, "one reservation and the same active hold", func() bool {
		st := s.stock()
		h := s.hold(hold.GetHoldId())
		return st.GetQuantityReserved() == 2 && st.GetQuantityAvailable() == 3 &&
			h.GetStatus() == holdsv1.HoldStatus_HOLD_STATUS_ACTIVE && h.GetPickupCode() == active.GetPickupCode()
	})
}

// ---------------------------------------------------------------------------

// scenario gives each test its own pharmacy, product and patient, so the tests
// share one running stack without seeing each other's stock or hold limits.
type scenario struct {
	t                       *testing.T
	pharmacy, product, user string
}

func newScenario(t *testing.T) *scenario {
	return &scenario{t: t, pharmacy: uuid(), product: uuid(), user: uuid()}
}

func uuid() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func (s *scenario) setStock(onHand int32) {
	s.t.Helper()
	_, err := stack.inventory.SetStock(ctx(s.t), &inventoryv1.SetStockRequest{
		PharmacyId: s.pharmacy, ProductId: s.product, QuantityOnHand: onHand, UpdatedByUserId: "saga-test",
	})
	if err != nil {
		s.t.Fatalf("SetStock: %v", err)
	}
}

func (s *scenario) stock() *inventoryv1.StockLevel {
	s.t.Helper()
	resp, err := stack.inventory.GetStock(ctx(s.t), &inventoryv1.GetStockRequest{PharmacyId: s.pharmacy, ProductId: s.product})
	if err != nil {
		s.t.Fatalf("GetStock: %v", err)
	}
	return resp.GetStock()
}

func (s *scenario) assertStock(onHand, reserved, available int32) {
	s.t.Helper()
	st := s.stock()
	if st.GetQuantityOnHand() != onHand || st.GetQuantityReserved() != reserved || st.GetQuantityAvailable() != available {
		s.t.Fatalf("stock on hand/reserved/available = %d/%d/%d, want %d/%d/%d",
			st.GetQuantityOnHand(), st.GetQuantityReserved(), st.GetQuantityAvailable(), onHand, reserved, available)
	}
}

func (s *scenario) createHold(quantity int32) *holdsv1.Hold {
	s.t.Helper()
	resp, err := stack.holds.CreateHold(ctx(s.t), &holdsv1.CreateHoldRequest{
		UserId: s.user, PharmacyId: s.pharmacy, ProductId: s.product, Quantity: quantity,
	})
	if err != nil {
		s.t.Fatalf("CreateHold: %v", err)
	}
	return resp.GetHold()
}

func (s *scenario) hold(id string) *holdsv1.Hold {
	s.t.Helper()
	resp, err := stack.holds.GetHold(ctx(s.t), &holdsv1.GetHoldRequest{HoldId: id})
	if err != nil {
		s.t.Fatalf("GetHold: %v", err)
	}
	return resp.GetHold()
}

func (s *scenario) waitStatus(id string, want holdsv1.HoldStatus) *holdsv1.Hold {
	s.t.Helper()
	var h *holdsv1.Hold
	eventually(s.t, 15*time.Second, fmt.Sprintf("hold %s to be %s", id, want), func() bool {
		h = s.hold(id)
		return h.GetStatus() == want
	})
	return h
}
