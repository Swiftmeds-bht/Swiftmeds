package app

import (
	"context"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
)

// SCRUM-141: after LinkPharmacy, a refreshed token carries the staff role and
// the pharmacy, so the gateway lets the account into its own pharmacy only.
func TestLinkPharmacyThenRefreshCarriesRoleAndPharmacy(t *testing.T) {
	repo := newFakeRepository()
	// The real clock: the JWT library checks expiry against it.
	svc := newTestService(repo, time.Now)
	ctx := context.Background()

	user, err := svc.Register(ctx, "owner@example.bt", "correct-horse-battery", domain.RolePatient)
	if err != nil {
		t.Fatal(err)
	}
	_, tokens, err := svc.Login(ctx, "owner@example.bt", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	before, err := svc.ValidateToken(ctx, tokens.AccessToken)
	if err != nil || before.Role != domain.RolePatient || before.PharmacyID != "" {
		t.Fatalf("before link: %+v err=%v", before, err)
	}

	if _, err := svc.LinkPharmacy(ctx, user.ID, "ph-new"); err != nil {
		t.Fatal(err)
	}
	refreshed, err := svc.Refresh(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	after, err := svc.ValidateToken(ctx, refreshed.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if after.Role != domain.RolePharmacist || after.PharmacyID != "ph-new" {
		t.Fatalf("after link: %+v", after)
	}
}

func TestLinkPharmacyNeedsBothIDs(t *testing.T) {
	svc := newTestService(newFakeRepository(), time.Now)
	if _, err := svc.LinkPharmacy(context.Background(), "u1", ""); err != domain.ErrMissingLink {
		t.Fatalf("err = %v, want ErrMissingLink", err)
	}
}
