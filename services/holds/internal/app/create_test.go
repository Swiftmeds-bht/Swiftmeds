package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
)

func TestCreateHoldStoresAPendingHold(t *testing.T) {
	svc, repo, _ := newTestService(t)

	h, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 2)

	if err != nil {
		t.Fatal(err)
	}
	if h.ID != "h-1" || h.UserID != "u-1" || h.PharmacyID != "ph-1" || h.ProductID != "med-1" || h.Quantity != 2 {
		t.Errorf("fields were not kept: %+v", h)
	}
	if h.Status != domain.Pending || !h.CreatedAt.Equal(t0) {
		t.Errorf("status = %s, created_at = %v, want PENDING at %v", h.Status, h.CreatedAt, t0)
	}
	if h.PickupCode != "" || !h.ExpiresAt.IsZero() {
		t.Errorf("pickup code = %q, expires_at = %v, want neither until the hold is ACTIVE", h.PickupCode, h.ExpiresAt)
	}
	stored, err := repo.Get(context.Background(), "h-1")
	if err != nil || stored.Status != domain.Pending {
		t.Errorf("stored hold = %+v, err = %v, want a PENDING hold", stored, err)
	}
}

func TestCreateHoldRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name                    string
		user, pharmacy, product string
		quantity                int
		wantErr                 error
	}{
		{"missing patient", "", "ph-1", "med-1", 1, domain.ErrMissingUserID},
		{"missing pharmacy", "u-1", "", "med-1", 1, domain.ErrMissingPharmacyID},
		{"missing product", "u-1", "ph-1", "", 1, domain.ErrMissingProductID},
		{"quantity too small", "u-1", "ph-1", "med-1", 0, domain.ErrInvalidQuantity},
		{"quantity too large", "u-1", "ph-1", "med-1", 6, domain.ErrInvalidQuantity},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, repo, _ := newTestService(t)

			_, err := svc.CreateHold(context.Background(), c.user, c.pharmacy, c.product, c.quantity)

			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if _, err := repo.Get(context.Background(), "h-1"); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("a hold was stored for an invalid request: err = %v", err)
			}
		})
	}
}

func TestCreateHoldFailsWhenNoIDCanBeMade(t *testing.T) {
	svc, _, _ := newTestService(t, WithIDGenerator(func() (string, error) { return "", errBoom }))

	_, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 1)

	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestCreateHoldReportsARepositoryError(t *testing.T) {
	svc := NewService(errRepo{}, testConfig(), (&clock{t: t0}).now)

	_, err := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 1)

	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}

func TestAPatientCannotHaveMoreThanThreeOpenHolds(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	for i := 0; i < 3; i++ {
		if _, err := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1); err != nil {
			t.Fatalf("hold %d: %v", i+1, err)
		}
	}

	_, err := svc.CreateHold(ctx, "u-1", "ph-2", "med-2", 1)

	if !errors.Is(err, domain.ErrLimitReached) {
		t.Fatalf("fourth hold: err = %v, want ErrLimitReached", err)
	}
	if _, err := svc.CreateHold(ctx, "u-2", "ph-1", "med-1", 1); err != nil {
		t.Errorf("another patient was stopped by u-1's limit: %v", err)
	}
}

func TestACancelledHoldFreesAPlaceUnderTheLimit(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newTestService(t)
	var first domain.Hold
	for i := 0; i < 3; i++ {
		h, err := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = h
		}
	}
	if _, err := svc.CancelHold(ctx, first.ID, "u-1", "changed my mind"); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1); err != nil {
		t.Fatalf("after cancelling: %v", err)
	}
}

func TestGetHold(t *testing.T) {
	svc, _, _ := newTestService(t)
	created, _ := svc.CreateHold(context.Background(), "u-1", "ph-1", "med-1", 1)

	got, err := svc.GetHold(context.Background(), created.ID)
	if err != nil || got.ID != created.ID {
		t.Fatalf("got (%+v, %v), want the created hold", got, err)
	}
	if _, err := svc.GetHold(context.Background(), "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown id: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.GetHold(context.Background(), ""); !errors.Is(err, domain.ErrMissingHoldID) {
		t.Errorf("empty id: err = %v, want ErrMissingHoldID", err)
	}
}

func TestDefaultGeneratorsMakeAUUIDAndASixCharacterCode(t *testing.T) {
	ctx := context.Background()
	svc := NewService(memory.NewRepository(), testConfig(), (&clock{t: t0}).now)

	h, err := svc.CreateHold(ctx, "u-1", "ph-1", "med-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(h.ID, "-")
	if len(h.ID) != 36 || len(parts) != 5 || h.ID[14] != '4' || !strings.ContainsRune("89ab", rune(h.ID[19])) {
		t.Errorf("hold id %q is not a version 4 UUID", h.ID)
	}

	active, err := svc.ReserveHold(ctx, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(active.PickupCode) != domain.PickupCodeLength {
		t.Errorf("pickup code %q has %d characters, want %d", active.PickupCode, len(active.PickupCode), domain.PickupCodeLength)
	}
}
