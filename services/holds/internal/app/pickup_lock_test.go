package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

const wrongCode = "ZZZ999"

func TestCollectLocksAfterTooManyWrongCodes(t *testing.T) {
	svc, inv, _, _ := sagaService(t)
	h := activeHold(t, svc)
	ctx := context.Background()

	for i := 1; i <= domain.MaxPickupAttempts; i++ {
		if _, err := svc.CollectHold(ctx, h.ID, "ph-1", wrongCode); !errors.Is(err, domain.ErrWrongPickupCode) {
			t.Fatalf("wrong code %d: err = %v, want ErrWrongPickupCode", i, err)
		}
		if got := mustGet(t, svc, h.ID).PickupAttempts; got != i {
			t.Fatalf("after wrong code %d: attempts = %d", i, got)
		}
	}

	// The 6th try is refused even with the right code.
	if _, err := svc.CollectHold(ctx, h.ID, "ph-1", testCode); !errors.Is(err, domain.ErrPickupLocked) {
		t.Fatalf("right code after %d wrong ones: err = %v, want ErrPickupLocked", domain.MaxPickupAttempts, err)
	}
	if _, err := svc.CollectHold(ctx, h.ID, "ph-1", wrongCode); !errors.Is(err, domain.ErrPickupLocked) {
		t.Fatalf("wrong code on a locked hold: err = %v, want ErrPickupLocked", err)
	}
	got := mustGet(t, svc, h.ID)
	if got.Status != domain.Active || got.PickupAttempts != domain.MaxPickupAttempts {
		t.Fatalf("hold = %s with %d attempts, want ACTIVE with %d", got.Status, got.PickupAttempts, domain.MaxPickupAttempts)
	}
	if inv.count("commit") != 0 {
		t.Error("a locked hold must not reach Inventory")
	}
}

func TestCollectStillWorksBeforeTheLimit(t *testing.T) {
	svc, inv, _, _ := sagaService(t)
	h := activeHold(t, svc)
	ctx := context.Background()
	for range domain.MaxPickupAttempts - 1 {
		_, _ = svc.CollectHold(ctx, h.ID, "ph-1", wrongCode)
	}
	got, err := svc.CollectHold(ctx, h.ID, "ph-1", testCode)
	if err != nil || got.Status != domain.Collected {
		t.Fatalf("collect after %d wrong codes = %s, %v; want COLLECTED", domain.MaxPickupAttempts-1, got.Status, err)
	}
	if inv.count("commit") != 1 {
		t.Errorf("commits = %d, want 1", inv.count("commit"))
	}
}

// Other refusals (wrong pharmacy, missing code) are not guesses and are not counted.
func TestCollectCountsOnlyWrongCodes(t *testing.T) {
	svc, _, _, _ := sagaService(t)
	h := activeHold(t, svc)
	ctx := context.Background()
	_, _ = svc.CollectHold(ctx, h.ID, "ph-other", wrongCode)
	_, _ = svc.CollectHold(ctx, h.ID, "ph-1", "")
	if got := mustGet(t, svc, h.ID).PickupAttempts; got != 0 {
		t.Fatalf("attempts = %d, want 0", got)
	}
}

// recordFails cannot count wrong codes.
type recordFails struct{ *memory.Repository }

func (recordFails) RecordWrongPickupCode(context.Context, string) (int, error) { return 0, errBoom }

func TestCollectFailsClosedWhenAWrongCodeCannotBeCounted(t *testing.T) {
	repo := recordFails{memory.NewRepository()}
	clk := &clock{t: t0}
	svc := NewService(repo, testConfig(), clk.now, WithCodeGenerator(always(testCode)))
	h := activeHold(t, svc)

	_, err := svc.CollectHold(context.Background(), h.ID, "ph-1", wrongCode)
	if !errors.Is(err, errBoom) || errors.Is(err, domain.ErrWrongPickupCode) {
		t.Fatalf("err = %v, want the counting error and not a plain wrong-code answer", err)
	}
}

// conflictOnRecord says the hold stopped being ACTIVE while the wrong code was counted.
type conflictOnRecord struct{ *memory.Repository }

func (conflictOnRecord) RecordWrongPickupCode(context.Context, string) (int, error) {
	return 0, domain.ErrConflict
}

func TestCollectWrongCodeOnAHoldThatJustEnded(t *testing.T) {
	repo := conflictOnRecord{memory.NewRepository()}
	svc := NewService(repo, testConfig(), (&clock{t: t0}).now, WithCodeGenerator(always(testCode)))
	h := activeHold(t, svc)
	if _, err := svc.CollectHold(context.Background(), h.ID, "ph-1", wrongCode); !errors.Is(err, domain.ErrWrongPickupCode) {
		t.Fatalf("err = %v, want ErrWrongPickupCode", err)
	}
}

func TestCreateHoldChecksThatThePharmacyStocksTheMedicine(t *testing.T) {
	for _, tc := range []struct {
		name     string
		checkErr error
	}{
		{"unknown pharmacy or product", domain.ErrNotStocked},
		{"inventory unreachable", domain.ErrInventoryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, inv, ev, _ := sagaService(t)
			inv.checkErr = tc.checkErr

			_, err := svc.CreateHold(context.Background(), "u-1", "ph-404", "med-404", 1)
			if !errors.Is(err, tc.checkErr) {
				t.Fatalf("err = %v, want %v", err, tc.checkErr)
			}
			if len(ev.requested) != 0 {
				t.Error("no reservation may be requested")
			}
			list, _ := svc.ListHolds(context.Background(), ListQuery{UserID: "u-1"})
			if len(list.Holds) != 0 {
				t.Errorf("stored %d holds, want none", len(list.Holds))
			}
		})
	}
}
