package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
)

func newRegisterService() *Service {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	next := 0
	return NewService(memory.NewRepository(), func() time.Time { return now }, func() string {
		next++
		return "ph-" + string(rune('0'+next))
	})
}

func paro(owner, license string) RegisterInput {
	return RegisterInput{
		OwnerID: owner, Name: "Paro Pharmacy", LicenseNumber: license,
		Phone: " +975 8 271 234 ", Address: " Main Street, Paro ", Dzongkhag: "Paro",
		Location: domain.GeoPoint{Latitude: 27.43, Longitude: 89.41},
	}
}

// SCRUM-141: the gateway may send the same registration again after a lost
// answer; the owner must get the same pharmacy, not a second one.
func TestRegisterIsIdempotentPerOwner(t *testing.T) {
	svc := newRegisterService()
	ctx := context.Background()

	first, err := svc.Register(ctx, paro("owner-1", "LIC-1"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Phone != "+975 8 271 234" || first.Address != "Main Street, Paro" {
		t.Fatalf("contact not trimmed: phone=%q address=%q", first.Phone, first.Address)
	}
	again, err := svc.Register(ctx, paro("owner-1", "LIC-1"))
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Fatalf("second registration made %s, want the first pharmacy %s", again.ID, first.ID)
	}
	all, err := svc.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("%d pharmacies stored, want 1", len(all))
	}
}

func TestRegisterRefusesAnotherOwnersLicence(t *testing.T) {
	svc := newRegisterService()
	ctx := context.Background()
	if _, err := svc.Register(ctx, paro("owner-1", "LIC-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, paro("owner-2", "LIC-1")); !errors.Is(err, domain.ErrLicenseTaken) {
		t.Fatalf("err = %v, want ErrLicenseTaken", err)
	}
}

func TestRegisterValidatesBeforeLookingUpTheOwner(t *testing.T) {
	svc := newRegisterService()
	in := paro("owner-1", "")
	if _, err := svc.Register(context.Background(), in); !errors.Is(err, domain.ErrMissingLicense) {
		t.Fatalf("err = %v, want ErrMissingLicense", err)
	}
}
