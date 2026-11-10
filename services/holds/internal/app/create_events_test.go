package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

func TestCreateHoldPublishesHoldRequested(t *testing.T) {
	svc, _, ev, _ := sagaService(t)

	h, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 2)

	if err != nil {
		t.Fatal(err)
	}
	if len(ev.requested) != 1 {
		t.Fatalf("hold.requested was published %d times, want 1", len(ev.requested))
	}
	got := ev.requested[0]
	if got.ID != h.ID || got.UserID != "u-1" || got.PharmacyID != "ph-1" || got.ProductID != "med-1" || got.Quantity != 2 {
		t.Errorf("event carries %+v, want the new hold %+v", got, h)
	}
	if h.Status != domain.Pending || mustGet(t, svc, h.ID).Status != domain.Pending {
		t.Error("the hold must be PENDING until Inventory answers")
	}
	if len(ev.confirmed) != 0 {
		t.Error("hold.confirmed must not be sent before the stock is reserved")
	}
}

func TestCreateHoldRejectsTheHoldWhenTheEventCannotBeSent(t *testing.T) {
	svc, _, ev, _ := sagaService(t)
	ev.errRequested = errBoom

	_, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 2)

	if !errors.Is(err, domain.ErrRequestNotSent) || !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want ErrRequestNotSent wrapping the publish error", err)
	}
	h := mustGet(t, svc, "h-1")
	if h.Status != domain.Rejected || h.StatusReason != reasonRequestNotSent {
		t.Errorf("hold is %s (%q), want REJECTED with the request-not-sent reason", h.Status, h.StatusReason)
	}
}

// A hold that could not be requested must not keep using up the patient's limit.
func TestAHoldRejectedForANotSentEventDoesNotCountAgainstTheLimit(t *testing.T) {
	svc, _, ev, _ := sagaService(t)
	ev.errRequested = errBoom
	for range 4 { // the limit is 3
		if _, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 1); !errors.Is(err, domain.ErrRequestNotSent) {
			t.Fatalf("err = %v, want ErrRequestNotSent (and not the limit)", err)
		}
	}

	ev.errRequested = nil
	if _, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 1); err != nil {
		t.Fatalf("err = %v, want the next try to work", err)
	}
}

func TestCreateHoldReportsBothFailuresWhenTheHoldCannotBeRejectedEither(t *testing.T) {
	repo := &hookRepo{HoldRepository: memory.NewRepository()}
	inv, ev := &fakeInventory{}, &fakeEvents{errRequested: errBoom}
	svc := NewService(repo, testConfig(), (&clock{t: t0}).now, WithInventory(inv), WithEvents(ev),
		WithIDGenerator(sequence("h-1")), WithCodeGenerator(always(testCode)))
	// Someone cancels the hold between our read and our write, so rejecting it fails.
	repo.beforeUpdate = func() {
		h, _ := repo.Get(context.Background(), "h-1")
		cancelled, _ := h.Cancel(t0, "x")
		_ = repo.HoldRepository.Update(context.Background(), cancelled, domain.Pending)
	}

	_, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 1)

	if !errors.Is(err, domain.ErrRequestNotSent) || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want both ErrRequestNotSent and ErrConflict", err)
	}
}

func TestCreateHoldPublishesNothingWhenItFails(t *testing.T) {
	svc, _, ev, _ := sagaService(t)
	ctx := context.Background()

	if _, err := svc.CreateHold(ctx, "", "ph-1", "med-1", 1); !errors.Is(err, domain.ErrMissingUserID) {
		t.Fatalf("err = %v, want ErrMissingUserID", err)
	}
	for range 3 {
		if _, err := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1); err != nil {
			t.Fatal(err)
		}
	}
	ev.requested = nil
	if _, err := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1); !errors.Is(err, domain.ErrLimitReached) {
		t.Fatalf("err = %v, want ErrLimitReached", err)
	}

	if len(ev.requested) != 0 {
		t.Errorf("%d events were published for requests that failed", len(ev.requested))
	}
}
