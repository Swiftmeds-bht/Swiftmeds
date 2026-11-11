//go:build e2e

// Package integration holds Holds' end-to-end tests. They run the reservation
// saga against a live stack (Holds, Inventory, RabbitMQ and Postgres) and check
// the stock numbers in Inventory after every step. They are skipped by the
// normal test run: use the "e2e" build tag.
//
// With the Docker Compose stack running (see README.md):
//
//	go test -tags e2e -count=1 -v ./test/integration/
//
// Calls go through Traefik on localhost:50051, which routes by :authority.
// Without Traefik, point the test at the services directly:
//
//	E2E_HOLDS_ADDR=localhost:50065 E2E_INVENTORY_ADDR=localhost:50056
package integration

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
)

const (
	waitFor     = 20 * time.Second // how long the saga may take to settle
	callTimeout = 5 * time.Second
)

type stack struct {
	holds     holdsv1.HoldsServiceClient
	inventory inventoryv1.InventoryServiceClient
}

func address(specific string) string {
	if v := os.Getenv(specific); v != "" {
		return v
	}
	if v := os.Getenv("E2E_UPSTREAM"); v != "" {
		return v
	}
	return "localhost:50051"
}

func dial(t *testing.T, addr, authority string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority(authority))
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func newStack(t *testing.T) *stack {
	t.Helper()
	return &stack{
		holds:     holdsv1.NewHoldsServiceClient(dial(t, address("E2E_HOLDS_ADDR"), "holds")),
		inventory: inventoryv1.NewInventoryServiceClient(dial(t, address("E2E_INVENTORY_ADDR"), "inventory")),
	}
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), callTimeout)
	t.Cleanup(cancel)
	return c
}

// newUUID returns a random UUID, so every test run uses its own pharmacy, product and patient.
func newUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// scenario is one pharmacy, product and patient with some stock on the shelf.
type scenario struct {
	*stack
	pharmacy, product, user string
}

func newScenario(t *testing.T, onHand int32) *scenario {
	t.Helper()
	s := &scenario{stack: newStack(t), pharmacy: newUUID(t), product: newUUID(t), user: newUUID(t)}
	_, err := s.inventory.SetStock(ctx(t), &inventoryv1.SetStockRequest{
		PharmacyId: s.pharmacy, ProductId: s.product, QuantityOnHand: onHand, UpdatedByUserId: s.user,
	})
	if err != nil {
		t.Fatalf("set stock: %v", err)
	}
	return s
}

func (s *scenario) stock(t *testing.T) *inventoryv1.StockLevel {
	t.Helper()
	res, err := s.inventory.GetStock(ctx(t), &inventoryv1.GetStockRequest{PharmacyId: s.pharmacy, ProductId: s.product})
	if err != nil {
		t.Fatalf("get stock: %v", err)
	}
	return res.GetStock()
}

func (s *scenario) createHold(t *testing.T, quantity int32) *holdsv1.Hold {
	t.Helper()
	res, err := s.holds.CreateHold(ctx(t), &holdsv1.CreateHoldRequest{
		UserId: s.user, PharmacyId: s.pharmacy, ProductId: s.product, Quantity: quantity,
	})
	if err != nil {
		t.Fatalf("create hold: %v", err)
	}
	return res.GetHold()
}

func (s *scenario) hold(t *testing.T, id string) *holdsv1.Hold {
	t.Helper()
	res, err := s.holds.GetHold(ctx(t), &holdsv1.GetHoldRequest{HoldId: id})
	if err != nil {
		t.Fatalf("get hold: %v", err)
	}
	return res.GetHold()
}

// eventually repeats check until it returns "" (success) or waitFor passes.
func eventually(t *testing.T, what string, check func() string) {
	t.Helper()
	deadline := time.Now().Add(waitFor)
	for {
		problem := check()
		if problem == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: still not true after %v (last: %s)", what, waitFor, problem)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *scenario) waitForStatus(t *testing.T, id string, want holdsv1.HoldStatus) *holdsv1.Hold {
	t.Helper()
	var h *holdsv1.Hold
	eventually(t, "hold "+want.String(), func() string {
		h = s.hold(t, id)
		if h.GetStatus() != want {
			return "status is " + h.GetStatus().String()
		}
		return ""
	})
	return h
}

func (s *scenario) waitForStock(t *testing.T, onHand, reserved int32) {
	t.Helper()
	eventually(t, fmt.Sprintf("stock on_hand=%d reserved=%d", onHand, reserved), func() string {
		st := s.stock(t)
		if st.GetQuantityOnHand() != onHand || st.GetQuantityReserved() != reserved {
			return fmt.Sprintf("on_hand=%d reserved=%d", st.GetQuantityOnHand(), st.GetQuantityReserved())
		}
		return ""
	})
}

func wantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %v (%v), want %v", got, err, want)
	}
}

// CreateHold -> hold.requested -> Inventory reserves -> stock.reserved -> ACTIVE
// with a pickup code -> collect (wrong code refused, right code works, twice fails)
// -> Inventory commits the stock.
func TestSagaReserveAndCollect(t *testing.T) {
	s := newScenario(t, 10)

	created := s.createHold(t, 2)
	if created.GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_PENDING {
		t.Fatalf("a new hold is %s, want PENDING", created.GetStatus())
	}

	active := s.waitForStatus(t, created.GetHoldId(), holdsv1.HoldStatus_HOLD_STATUS_ACTIVE)
	code := active.GetPickupCode()
	if code == "" || active.GetExpiresAt() == nil {
		t.Fatalf("an ACTIVE hold needs a pickup code and an expiry time, got %v", active)
	}
	if st := s.stock(t); st.GetQuantityReserved() != 2 || st.GetQuantityAvailable() != 8 {
		t.Fatalf("stock = %v, want 2 reserved and 8 available", st)
	}

	collect := func(pickupCode string) error {
		_, err := s.holds.CollectHold(ctx(t), &holdsv1.CollectHoldRequest{
			HoldId: created.GetHoldId(), PharmacyId: s.pharmacy, PickupCode: pickupCode,
		})
		return err
	}
	wantCode(t, collect("ZZZZZZ"), codes.PermissionDenied) // wrong pickup code is rejected
	if h := s.hold(t, created.GetHoldId()); h.GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_ACTIVE {
		t.Fatalf("a wrong code changed the hold to %s", h.GetStatus())
	}
	if st := s.stock(t); st.GetQuantityOnHand() != 10 || st.GetQuantityReserved() != 2 {
		t.Fatalf("a wrong code touched the stock: %v", st)
	}

	if err := collect(code); err != nil {
		t.Fatalf("collect with the right code: %v", err)
	}
	s.waitForStock(t, 8, 0) // the two units left the shelf for good
	if h := s.hold(t, created.GetHoldId()); h.GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_COLLECTED {
		t.Fatalf("hold is %s, want COLLECTED", h.GetStatus())
	}

	wantCode(t, collect(code), codes.FailedPrecondition) // collecting twice fails
	if st := s.stock(t); st.GetQuantityOnHand() != 8 || st.GetQuantityReserved() != 0 {
		t.Fatalf("collecting twice changed the stock: %v", st)
	}
}

// Not enough stock: Inventory answers stock.unavailable and the hold is REJECTED.
func TestSagaStockUnavailable(t *testing.T) {
	s := newScenario(t, 1)

	created := s.createHold(t, 3)
	rejected := s.waitForStatus(t, created.GetHoldId(), holdsv1.HoldStatus_HOLD_STATUS_REJECTED)

	if rejected.GetPickupCode() != "" {
		t.Errorf("a rejected hold has pickup code %q, want none", rejected.GetPickupCode())
	}
	if st := s.stock(t); st.GetQuantityOnHand() != 1 || st.GetQuantityReserved() != 0 {
		t.Errorf("stock = %v, want it untouched", st)
	}
}

// Compensation: cancelling an ACTIVE hold gives its stock back to Inventory.
func TestSagaCancelReleasesTheStock(t *testing.T) {
	s := newScenario(t, 10)
	created := s.createHold(t, 4)
	s.waitForStatus(t, created.GetHoldId(), holdsv1.HoldStatus_HOLD_STATUS_ACTIVE)
	s.waitForStock(t, 10, 4)

	_, err := s.holds.CancelHold(ctx(t), &holdsv1.CancelHoldRequest{HoldId: created.GetHoldId(), UserId: s.user})

	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if h := s.hold(t, created.GetHoldId()); h.GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_CANCELLED {
		t.Fatalf("hold is %s, want CANCELLED", h.GetStatus())
	}
	s.waitForStock(t, 10, 0)
}

// Cancelling straight after creating races with Inventory's reservation. Whichever
// lands first, the stock must end up released and the hold must stay CANCELLED.
func TestSagaCancelRightAfterCreateNeverLeaksStock(t *testing.T) {
	s := newScenario(t, 10)

	created := s.createHold(t, 3)
	_, err := s.holds.CancelHold(ctx(t), &holdsv1.CancelHoldRequest{HoldId: created.GetHoldId(), UserId: s.user})
	if err != nil {
		// The reservation may have won the race and the hold may already be ACTIVE: cancel again.
		_, err = s.holds.CancelHold(ctx(t), &holdsv1.CancelHoldRequest{HoldId: created.GetHoldId(), UserId: s.user})
		if err != nil {
			t.Fatalf("cancel: %v", err)
		}
	}

	s.waitForStock(t, 10, 0)
	time.Sleep(time.Second) // a late stock.reserved must not bring the hold back to life
	if h := s.hold(t, created.GetHoldId()); h.GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_CANCELLED {
		t.Fatalf("hold is %s, want CANCELLED", h.GetStatus())
	}
	s.waitForStock(t, 10, 0)
}

// Compensation: an expired hold gives its stock back. This needs a short hold
// time, so it only runs when the stack was started with HOLDS_HOLD_DURATION=5s.
func TestSagaExpireReleasesTheStock(t *testing.T) {
	s := newScenario(t, 10)
	created := s.createHold(t, 5)
	active := s.waitForStatus(t, created.GetHoldId(), holdsv1.HoldStatus_HOLD_STATUS_ACTIVE)
	expiresAt := active.GetExpiresAt().AsTime()
	if time.Until(expiresAt) > 30*time.Second {
		t.Skip("start the stack with HOLDS_HOLD_DURATION=5s to test expiry")
	}
	s.waitForStock(t, 10, 5)

	_, err := s.holds.ExpireHold(ctx(t), &holdsv1.ExpireHoldRequest{HoldId: created.GetHoldId()})
	wantCode(t, err, codes.FailedPrecondition) // too early
	time.Sleep(time.Until(expiresAt) + 500*time.Millisecond)
	if _, err := s.holds.ExpireHold(ctx(t), &holdsv1.ExpireHoldRequest{HoldId: created.GetHoldId()}); err != nil {
		t.Fatalf("expire: %v", err)
	}

	if h := s.hold(t, created.GetHoldId()); h.GetStatus() != holdsv1.HoldStatus_HOLD_STATUS_EXPIRED {
		t.Fatalf("hold is %s, want EXPIRED", h.GetStatus())
	}
	s.waitForStock(t, 10, 0)
}
