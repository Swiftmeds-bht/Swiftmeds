package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// ---- stock.reserved ------------------------------------------------------------------------

func TestStockReservedMakesTheHoldActiveAndConfirmsIt(t *testing.T) {
	svc, inv, ev, _ := sagaService(t)
	h := pendingHold(t, svc, ev)

	outcome, err := svc.HandleStockReserved(context.Background(), h.ID)

	if err != nil || outcome != OutcomeConfirmed {
		t.Fatalf("got (%s, %v), want (confirmed, nil)", outcome, err)
	}
	got := mustGet(t, svc, h.ID)
	if got.Status != domain.Active || got.PickupCode != testCode || !got.ExpiresAt.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("hold = %+v, want ACTIVE with code %s expiring at %v", got, testCode, t0.Add(2*time.Hour))
	}
	if len(ev.confirmed) != 1 || ev.confirmed[0].ID != h.ID || ev.confirmed[0].PickupCode != testCode {
		t.Errorf("hold.confirmed events = %+v, want one with the pickup code", ev.confirmed)
	}
	if len(inv.calls) != 0 {
		t.Errorf("Inventory was called %v, want no call", inv.calls)
	}
}

func TestDuplicateStockReservedChangesNothingAndConfirmsAgain(t *testing.T) {
	svc, inv, ev, clk := sagaService(t)
	h := pendingHold(t, svc, ev)
	if _, err := svc.HandleStockReserved(context.Background(), h.ID); err != nil {
		t.Fatal(err)
	}
	first := mustGet(t, svc, h.ID)
	clk.advance(30 * time.Minute) // a re-reservation would move the expiry time

	outcome, err := svc.HandleStockReserved(context.Background(), h.ID)

	if err != nil || outcome != OutcomeAlreadyConfirmed {
		t.Fatalf("got (%s, %v), want (already_confirmed, nil)", outcome, err)
	}
	if got := mustGet(t, svc, h.ID); got != first {
		t.Errorf("hold changed on a duplicate event:\n got %+v\nwant %+v", got, first)
	}
	if len(ev.confirmed) != 2 || ev.confirmed[0] != ev.confirmed[1] {
		t.Errorf("want hold.confirmed twice with identical content, got %+v", ev.confirmed)
	}
	if len(inv.calls) != 0 {
		t.Errorf("Inventory was called %v, want no call", inv.calls)
	}
}

// If hold.confirmed could not be sent, the retry of stock.reserved must send it.
func TestStockReservedRetryResendsAFailedConfirmation(t *testing.T) {
	svc, _, ev, _ := sagaService(t)
	h := pendingHold(t, svc, ev)
	ev.errConfirmed = errBoom

	if _, err := svc.HandleStockReserved(context.Background(), h.ID); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want the publish error (so the consumer retries)", err)
	}
	if mustGet(t, svc, h.ID).Status != domain.Active {
		t.Fatal("the hold must stay ACTIVE: the stock is reserved")
	}

	ev.errConfirmed = nil
	outcome, err := svc.HandleStockReserved(context.Background(), h.ID)

	if err != nil || outcome != OutcomeAlreadyConfirmed {
		t.Fatalf("retry = (%s, %v), want (already_confirmed, nil)", outcome, err)
	}
	if len(ev.confirmed) != 2 {
		t.Errorf("hold.confirmed attempts = %d, want 2 (failed, then sent)", len(ev.confirmed))
	}
}

// The patient cancelled while Inventory was reserving: the late reservation is given back.
func TestStockReservedForAnEndedHoldReleasesTheStock(t *testing.T) {
	cases := map[string]func(t *testing.T, svc *Service, ev *fakeEvents) domain.Hold{
		"cancelled": func(t *testing.T, svc *Service, ev *fakeEvents) domain.Hold {
			h := pendingHold(t, svc, ev)
			if _, err := svc.CancelHold(context.Background(), h.ID, "u-1", "changed my mind"); err != nil {
				t.Fatal(err)
			}
			return mustGet(t, svc, h.ID)
		},
		"rejected": func(t *testing.T, svc *Service, ev *fakeEvents) domain.Hold {
			h := pendingHold(t, svc, ev)
			if _, err := svc.RejectHold(context.Background(), h.ID, "no stock"); err != nil {
				t.Fatal(err)
			}
			return mustGet(t, svc, h.ID)
		},
		"expired": func(t *testing.T, svc *Service, ev *fakeEvents) domain.Hold {
			h := pendingHold(t, svc, ev)
			if _, err := svc.ReserveHold(context.Background(), h.ID); err != nil {
				t.Fatal(err)
			}
			svc.now = func() time.Time { return t0.Add(3 * time.Hour) }
			if _, err := svc.ExpireHold(context.Background(), h.ID); err != nil {
				t.Fatal(err)
			}
			return mustGet(t, svc, h.ID)
		},
	}
	for name, end := range cases {
		t.Run(name, func(t *testing.T) {
			svc, inv, ev, _ := sagaService(t)
			ended := end(t, svc, ev)
			inv.calls, ev.confirmed = nil, nil // forget what the setup did

			outcome, err := svc.HandleStockReserved(context.Background(), ended.ID)

			if err != nil || outcome != OutcomeReleased {
				t.Fatalf("got (%s, %v), want (released, nil)", outcome, err)
			}
			if len(inv.calls) != 1 || inv.calls[0] != (inventoryCall{"release", ended.ID}) {
				t.Errorf("Inventory calls = %v, want one release of %s", inv.calls, ended.ID)
			}
			if got := mustGet(t, svc, ended.ID); got != ended {
				t.Errorf("an ended hold changed: %+v", got)
			}
			if len(ev.confirmed) != 0 {
				t.Error("hold.confirmed was sent for a hold that has ended")
			}
		})
	}
}

func TestLateReservationIsReleasedOnRetryWhenInventoryWasDown(t *testing.T) {
	svc, inv, ev, _ := sagaService(t)
	h := pendingHold(t, svc, ev)
	if _, err := svc.CancelHold(context.Background(), h.ID, "u-1", "x"); err != nil {
		t.Fatal(err)
	}
	inv.err = domain.ErrInventoryUnavailable

	if _, err := svc.HandleStockReserved(context.Background(), h.ID); !errors.Is(err, domain.ErrInventoryUnavailable) {
		t.Fatalf("err = %v, want ErrInventoryUnavailable (so the consumer retries)", err)
	}

	inv.err = nil
	outcome, err := svc.HandleStockReserved(context.Background(), h.ID)
	if err != nil || outcome != OutcomeReleased {
		t.Fatalf("retry = (%s, %v), want (released, nil)", outcome, err)
	}
	if inv.count("release") != 2 {
		t.Errorf("release attempts = %d, want 2", inv.count("release"))
	}
}

func TestStockReservedForACollectedHoldDoesNothing(t *testing.T) {
	svc, inv, ev, _ := sagaService(t)
	h := activeHold(t, svc)
	if _, err := svc.CollectHold(context.Background(), h.ID, "ph-1", testCode); err != nil {
		t.Fatal(err)
	}
	inv.calls = nil

	outcome, err := svc.HandleStockReserved(context.Background(), h.ID)

	if err != nil || outcome != OutcomeIgnored {
		t.Fatalf("got (%s, %v), want (ignored, nil)", outcome, err)
	}
	if len(inv.calls) != 0 || len(ev.confirmed) != 0 {
		t.Errorf("a collected hold caused calls %v and events %v", inv.calls, ev.confirmed)
	}
}

func TestStockReservedForAnUnknownOrMissingHoldFails(t *testing.T) {
	svc, inv, ev, _ := sagaService(t)

	if _, err := svc.HandleStockReserved(context.Background(), "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown hold: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.HandleStockReserved(context.Background(), ""); !errors.Is(err, domain.ErrMissingHoldID) {
		t.Errorf("empty hold ID: err = %v, want ErrMissingHoldID", err)
	}
	if len(inv.calls) != 0 || len(ev.confirmed) != 0 {
		t.Error("nothing should be called or published for a hold that does not exist")
	}
}

// The patient cancels at the very moment the reservation is applied. The write
// loses, the event is retried, and the retry sees the cancelled hold and releases the stock.
func TestStockReservedThatLosesARaceWithCancelIsRetriedAndReleased(t *testing.T) {
	repo := &hookRepo{HoldRepository: memory.NewRepository()}
	inv, ev := &fakeInventory{}, &fakeEvents{}
	svc := NewService(repo, testConfig(), (&clock{t: t0}).now, WithInventory(inv), WithEvents(ev),
		WithIDGenerator(sequence("h-1")), WithCodeGenerator(always(testCode)))
	h := pendingHold(t, svc, ev)
	repo.beforeUpdate = func() {
		cancelled, _ := h.Cancel(t0, "changed my mind")
		_ = repo.HoldRepository.Update(context.Background(), cancelled, domain.Pending)
	}

	if _, err := svc.HandleStockReserved(context.Background(), h.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict (the consumer will retry)", err)
	}
	if len(ev.confirmed) != 0 {
		t.Fatal("hold.confirmed was sent for a hold that was just cancelled")
	}

	outcome, err := svc.HandleStockReserved(context.Background(), h.ID)
	if err != nil || outcome != OutcomeReleased {
		t.Fatalf("retry = (%s, %v), want (released, nil)", outcome, err)
	}
	if inv.count("release") != 1 {
		t.Errorf("release calls = %d, want 1", inv.count("release"))
	}
}

// ---- stock.unavailable ---------------------------------------------------------------------

func TestStockUnavailableRejectsThePendingHold(t *testing.T) {
	svc, inv, ev, _ := sagaService(t)
	h := pendingHold(t, svc, ev)

	outcome, err := svc.HandleStockUnavailable(context.Background(), h.ID)

	if err != nil || outcome != OutcomeRejected {
		t.Fatalf("got (%s, %v), want (rejected, nil)", outcome, err)
	}
	got := mustGet(t, svc, h.ID)
	if got.Status != domain.Rejected || got.StatusReason != reasonStockUnavailable || got.PickupCode != "" {
		t.Errorf("hold = %+v, want REJECTED (%q) with no pickup code", got, reasonStockUnavailable)
	}
	if len(inv.calls) != 0 || len(ev.confirmed) != 0 {
		t.Errorf("calls %v and events %v, want none: nothing was reserved", inv.calls, ev.confirmed)
	}
}

func TestDuplicateStockUnavailableChangesNothing(t *testing.T) {
	svc, _, ev, clk := sagaService(t)
	h := pendingHold(t, svc, ev)
	if _, err := svc.HandleStockUnavailable(context.Background(), h.ID); err != nil {
		t.Fatal(err)
	}
	first := mustGet(t, svc, h.ID)
	clk.advance(time.Hour)

	outcome, err := svc.HandleStockUnavailable(context.Background(), h.ID)

	if err != nil || outcome != OutcomeAlreadyRejected {
		t.Fatalf("got (%s, %v), want (already_rejected, nil)", outcome, err)
	}
	if got := mustGet(t, svc, h.ID); got != first {
		t.Errorf("hold changed on a duplicate event: %+v", got)
	}
}

func TestStockUnavailableIsIgnoredWhenItNoLongerApplies(t *testing.T) {
	t.Run("the patient already cancelled", func(t *testing.T) {
		svc, _, ev, _ := sagaService(t)
		h := pendingHold(t, svc, ev)
		if _, err := svc.CancelHold(context.Background(), h.ID, "u-1", "x"); err != nil {
			t.Fatal(err)
		}

		outcome, err := svc.HandleStockUnavailable(context.Background(), h.ID)

		if err != nil || outcome != OutcomeIgnored {
			t.Fatalf("got (%s, %v), want (ignored, nil)", outcome, err)
		}
		if mustGet(t, svc, h.ID).Status != domain.Cancelled {
			t.Error("a cancelled hold must stay CANCELLED")
		}
	})

	t.Run("the hold already has its stock", func(t *testing.T) {
		svc, inv, _, _ := sagaService(t)
		h := activeHold(t, svc)

		outcome, err := svc.HandleStockUnavailable(context.Background(), h.ID)

		if err != nil || outcome != OutcomeIgnored {
			t.Fatalf("got (%s, %v), want (ignored, nil)", outcome, err)
		}
		if got := mustGet(t, svc, h.ID); got.Status != domain.Active || got.PickupCode != testCode {
			t.Errorf("an ACTIVE hold changed: %+v", got)
		}
		if len(inv.calls) != 0 {
			t.Errorf("Inventory was called %v, want no call", inv.calls)
		}
	})
}

func TestStockUnavailableForAnUnknownHoldFails(t *testing.T) {
	svc, _, _, _ := sagaService(t)

	if _, err := svc.HandleStockUnavailable(context.Background(), "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestARejectedHoldFreesThePatientsSlot(t *testing.T) {
	svc, _, ev, _ := sagaService(t)
	ctx := context.Background()
	var first domain.Hold
	for i := range 3 {
		h, err := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = h
		}
	}
	ev.requested = nil
	if _, err := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1); !errors.Is(err, domain.ErrLimitReached) {
		t.Fatalf("err = %v, want ErrLimitReached", err)
	}

	if _, err := svc.HandleStockUnavailable(ctx, first.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1); err != nil {
		t.Fatalf("err = %v, want a new hold to be allowed", err)
	}
}
