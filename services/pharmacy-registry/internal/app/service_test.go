package app

import (
	"context"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
)

func TestRegistrationAndVerifiedOnlyList(t *testing.T) {
	repo := memory.NewRepository()
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	nextID := 0
	svc := NewService(repo, func() time.Time { return now }, func() string {
		nextID++
		return "ph-" + string(rune('0'+nextID))
	})

	created, err := svc.Register(context.Background(), RegisterInput{
		OwnerID: "owner-1", Name: "Paro Pharmacy", LicenseNumber: "LIC-1",
		Location: domain.GeoPoint{Latitude: 27.43, Longitude: 89.41},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != domain.StatusPending {
		t.Fatalf("status = %s, want PENDING", created.Status)
	}

	verified, err := svc.SetVerificationStatus(context.Background(), "ph-1", domain.StatusVerified, "")
	if err != nil {
		t.Fatal(err)
	}
	if verified.Status != domain.StatusVerified || verified.VerifiedAt == nil {
		t.Fatalf("verified pharmacy = %+v", verified)
	}

	other, err := svc.Register(context.Background(), RegisterInput{
		OwnerID: "owner-2", Name: "Thimphu Pharmacy", LicenseNumber: "LIC-2",
		Location: domain.GeoPoint{Latitude: 27.47, Longitude: 89.64},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetVerificationStatus(context.Background(), other.ID, domain.StatusRejected, "licence expired"); err != nil {
		t.Fatal(err)
	}

	got, err := svc.List(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "ph-1" {
		t.Fatalf("verified-only list = %+v, want only ph-1", got)
	}
}

func TestGetIncludesUnverifiedPharmacy(t *testing.T) {
	repo := memory.NewRepository()
	svc := NewService(repo, time.Now, func() string { return "ph-1" })
	if _, err := svc.Register(context.Background(), RegisterInput{
		OwnerID: "owner-1", Name: "Name", LicenseNumber: "LIC-1",
		Location: domain.GeoPoint{Latitude: 27, Longitude: 89},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(context.Background(), "ph-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusPending {
		t.Fatalf("status = %s, want PENDING", got.Status)
	}
}
