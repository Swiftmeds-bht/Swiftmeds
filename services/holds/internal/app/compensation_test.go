package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

// compService is a Service with a fake Inventory that notes what status the hold
// had at the moment of each Inventory call (to prove "Inventory first, then the hold").
func compService(t *testing.T) (*Service, *fakeInventory, *clock, *[]domain.Status) {
	t.Helper()
	inv := &fakeInventory{}
	svc, repo, clk := newTestService(t, WithInventory(inv), WithEvents(&fakeEvents{}))
	var seen []domain.Status
	inv.onCall = func(_, holdID string) {
		h, err := repo.Get(context.Background(), holdID)
		if err != nil {
			t.Errorf("hold %s not readable during the Inventory call: %v", holdID, err)
		}
		seen = append(seen, h.Status)
	}
	return svc, inv, clk, &seen
}

// ---- cancel ----------------------------------------------------------------------------------

func TestCancellingAnActiveHoldReleasesTheStockBeforeChangingTheHold(t *testing.T) {
	svc, inv, _, seen := compService(t)
	h := activeHold(t, svc)

	got, err := svc.CancelHold(context.Background(), h.ID, "u-1", "changed my mind")

	if err != nil || got.Status != domain.Cancelled {
		t.Fatalf("got (%+v, %v), want a CANCELLED hold", got, err)
	}
	if len(inv.calls) != 1 || inv.calls[0] != (inventoryCall{"release", h.ID}) {
		t.Fatalf("Inventory calls = %v, want one release of %s", inv.calls, h.ID)
	}
	if len(*seen) != 1 || (*seen)[0] != domain.Active {
		t.Errorf("hold was %v during the release, want ACTIVE (Inventory first, then the hold)", *seen)
	}
}

// Nothing is known to be reserved for a PENDING hold. If Inventory reserves a
// moment later, HandleStockReserved gives that stock back.
func TestCancellingAPendingHoldDoesNotCallInventory(t *testing.T) {
	svc, inv, _, _ := compService(t)
	h, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 1)
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.CancelHold(context.Background(), h.ID, "u-1", "changed my mind")

	if err != nil || got.Status != domain.Cancelled {
		t.Fatalf("got (%+v, %v), want a CANCELLED hold", got, err)
	}
	if len(inv.calls) != 0 {
		t.Errorf("Inventory calls = %v, want none", inv.calls)
	}
}

func TestCancelKeepsTheHoldActiveWhenInventoryFailsAndWorksOnRetry(t *testing.T) {
	svc, inv, _, _ := compService(t)
	h := activeHold(t, svc)
	inv.err = domain.ErrInventoryUnavailable

	_, err := svc.CancelHold(context.Background(), h.ID, "u-1", "x")

	if !errors.Is(err, domain.ErrInventoryUnavailable) {
		t.Fatalf("err = %v, want ErrInventoryUnavailable", err)
	}
	if got := mustGet(t, svc, h.ID); got.Status != domain.Active || got.PickupCode != testCode {
		t.Fatalf("hold = %+v, want it untouched (ACTIVE) so the stock is not leaked", got)
	}

	inv.err = nil
	if got, err := svc.CancelHold(context.Background(), h.ID, "u-1", "x"); err != nil || got.Status != domain.Cancelled {
		t.Fatalf("retry = (%+v, %v), want CANCELLED", got, err)
	}
	if inv.count("release") != 2 {
		t.Errorf("release attempts = %d, want 2", inv.count("release"))
	}
}

func TestCancelByAnotherPatientNeverReachesInventory(t *testing.T) {
	svc, inv, _, _ := compService(t)
	h := activeHold(t, svc)

	_, err := svc.CancelHold(context.Background(), h.ID, "u-2", "x")

	if !errors.Is(err, domain.ErrNotHoldOwner) {
		t.Fatalf("err = %v, want ErrNotHoldOwner", err)
	}
	if len(inv.calls) != 0 {
		t.Errorf("Inventory calls = %v, want none", inv.calls)
	}
}

func TestCancellingTwiceReleasesOnlyOnce(t *testing.T) {
	svc, inv, _, _ := compService(t)
	h := activeHold(t, svc)
	if _, err := svc.CancelHold(context.Background(), h.ID, "u-1", "x"); err != nil {
		t.Fatal(err)
	}

	_, err := svc.CancelHold(context.Background(), h.ID, "u-1", "x")

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	if inv.count("release") != 1 {
		t.Errorf("release calls = %d, want 1", inv.count("release"))
	}
}

// ---- expire ----------------------------------------------------------------------------------

func TestExpiringAHoldReleasesTheStockBeforeChangingTheHold(t *testing.T) {
	svc, inv, clk, seen := compService(t)
	h := activeHold(t, svc)
	clk.advance(2 * time.Hour) // exactly the expiry time

	got, err := svc.ExpireHold(context.Background(), h.ID)

	if err != nil || got.Status != domain.Expired {
		t.Fatalf("got (%+v, %v), want an EXPIRED hold", got, err)
	}
	if len(inv.calls) != 1 || inv.calls[0] != (inventoryCall{"release", h.ID}) {
		t.Fatalf("Inventory calls = %v, want one release of %s", inv.calls, h.ID)
	}
	if len(*seen) != 1 || (*seen)[0] != domain.Active {
		t.Errorf("hold was %v during the release, want ACTIVE", *seen)
	}
}

func TestExpireBeforeTheTimeIsUpDoesNotTouchInventory(t *testing.T) {
	svc, inv, clk, _ := compService(t)
	h := activeHold(t, svc)
	clk.advance(2*time.Hour - time.Nanosecond)

	_, err := svc.ExpireHold(context.Background(), h.ID)

	if !errors.Is(err, domain.ErrNotYetExpired) {
		t.Fatalf("err = %v, want ErrNotYetExpired", err)
	}
	if mustGet(t, svc, h.ID).Status != domain.Active {
		t.Error("an early expire changed the hold")
	}
	if len(inv.calls) != 0 {
		t.Errorf("Inventory calls = %v, want none", inv.calls)
	}
}

func TestExpireKeepsTheHoldActiveWhenInventoryFailsAndWorksOnRetry(t *testing.T) {
	svc, inv, clk, _ := compService(t)
	h := activeHold(t, svc)
	clk.advance(3 * time.Hour)
	inv.err = domain.ErrInventoryUnavailable

	_, err := svc.ExpireHold(context.Background(), h.ID)

	if !errors.Is(err, domain.ErrInventoryUnavailable) {
		t.Fatalf("err = %v, want ErrInventoryUnavailable", err)
	}
	if mustGet(t, svc, h.ID).Status != domain.Active {
		t.Fatal("the hold must stay ACTIVE, so the Expiry service calls again")
	}

	inv.err = nil
	if got, err := svc.ExpireHold(context.Background(), h.ID); err != nil || got.Status != domain.Expired {
		t.Fatalf("retry = (%+v, %v), want EXPIRED", got, err)
	}
}

func TestExpiringTwiceReleasesOnlyOnce(t *testing.T) {
	svc, inv, clk, _ := compService(t)
	h := activeHold(t, svc)
	clk.advance(3 * time.Hour)
	if _, err := svc.ExpireHold(context.Background(), h.ID); err != nil {
		t.Fatal(err)
	}

	got, err := svc.ExpireHold(context.Background(), h.ID)

	if err != nil || got.Status != domain.Expired {
		t.Fatalf("second call = (%+v, %v), want the same EXPIRED hold", got, err)
	}
	if inv.count("release") != 1 {
		t.Errorf("release calls = %d, want 1", inv.count("release"))
	}
}

// ---- collect ---------------------------------------------------------------------------------

func TestCollectingCommitsTheStockBeforeChangingTheHold(t *testing.T) {
	svc, inv, _, seen := compService(t)
	h := activeHold(t, svc)

	got, err := svc.CollectHold(context.Background(), h.ID, "ph-1", testCode)

	if err != nil || got.Status != domain.Collected {
		t.Fatalf("got (%+v, %v), want a COLLECTED hold", got, err)
	}
	if len(inv.calls) != 1 || inv.calls[0] != (inventoryCall{"commit", h.ID}) {
		t.Fatalf("Inventory calls = %v, want one commit of %s", inv.calls, h.ID)
	}
	if len(*seen) != 1 || (*seen)[0] != domain.Active {
		t.Errorf("hold was %v during the commit, want ACTIVE", *seen)
	}
}

// A request that is refused must never take stock out of Inventory.
func TestARefusedCollectNeverReachesInventory(t *testing.T) {
	cases := []struct {
		name       string
		pharmacyID string
		code       string
		advance    time.Duration
		wantErr    error
	}{
		{"wrong pickup code", "ph-1", "ZZZ999", 0, domain.ErrWrongPickupCode},
		{"no pickup code", "ph-1", "", 0, domain.ErrMissingPickupCode},
		{"another pharmacy", "ph-2", testCode, 0, domain.ErrWrongPharmacy},
		{"after the time ran out", "ph-1", testCode, 2 * time.Hour, domain.ErrHoldExpired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, inv, clk, _ := compService(t)
			h := activeHold(t, svc)
			clk.advance(c.advance)

			_, err := svc.CollectHold(context.Background(), h.ID, c.pharmacyID, c.code)

			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if len(inv.calls) != 0 {
				t.Errorf("Inventory calls = %v, want none", inv.calls)
			}
			if mustGet(t, svc, h.ID).Status != domain.Active {
				t.Error("a refused collect changed the hold")
			}
		})
	}
}

func TestCollectKeepsTheHoldActiveWhenInventoryFailsAndWorksOnRetry(t *testing.T) {
	svc, inv, _, _ := compService(t)
	h := activeHold(t, svc)
	inv.err = domain.ErrInventoryUnavailable

	_, err := svc.CollectHold(context.Background(), h.ID, "ph-1", testCode)

	if !errors.Is(err, domain.ErrInventoryUnavailable) {
		t.Fatalf("err = %v, want ErrInventoryUnavailable", err)
	}
	if mustGet(t, svc, h.ID).Status != domain.Active {
		t.Fatal("the hold must stay ACTIVE: the medicine was not handed over")
	}

	inv.err = nil
	if got, err := svc.CollectHold(context.Background(), h.ID, "ph-1", testCode); err != nil || got.Status != domain.Collected {
		t.Fatalf("retry = (%+v, %v), want COLLECTED", got, err)
	}
}

func TestCollectingTwiceFailsAndCommitsOnlyOnce(t *testing.T) {
	svc, inv, _, _ := compService(t)
	h := activeHold(t, svc)
	if _, err := svc.CollectHold(context.Background(), h.ID, "ph-1", testCode); err != nil {
		t.Fatal(err)
	}

	_, err := svc.CollectHold(context.Background(), h.ID, "ph-1", testCode)

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	if inv.count("commit") != 1 {
		t.Errorf("commit calls = %d, want 1", inv.count("commit"))
	}
}

// ---- reject ----------------------------------------------------------------------------------

func TestRejectingAHoldDoesNotCallInventory(t *testing.T) {
	svc, inv, _, _ := compService(t)
	h, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 1)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RejectHold(context.Background(), h.ID, "no stock"); err != nil {
		t.Fatal(err)
	}

	if len(inv.calls) != 0 {
		t.Errorf("Inventory calls = %v, want none: nothing was reserved", inv.calls)
	}
}
