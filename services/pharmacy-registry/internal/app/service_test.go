package app

import (
	"context"
	"errors"
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
		Dzongkhag: "Paro",
		Location:  domain.GeoPoint{Latitude: 27.43, Longitude: 89.41},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != domain.StatusPending {
		t.Fatalf("status = %s, want PENDING", created.Status)
	}
	if created.Dzongkhag != "Paro" {
		t.Fatalf("dzongkhag = %q, want %q", created.Dzongkhag, "Paro")
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

func TestUpdatePharmacyDetailsAndResubmitsRejected(t *testing.T) {
	repo := memory.NewRepository()
	svc := NewService(repo, time.Now, func() string { return "ph-1" })
	created, err := svc.Register(context.Background(), RegisterInput{
		OwnerID: "owner-1", Name: "Old Name", LicenseNumber: "LIC-1",
		Location: domain.GeoPoint{Latitude: 27, Longitude: 89},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetVerificationStatus(context.Background(), created.ID, domain.StatusRejected, "fix details"); err != nil {
		t.Fatal(err)
	}

	updated, err := svc.Update(context.Background(), created.ID, "owner-1", UpdateInput{
		Name: " New Name ", Phone: " 123 ", Address: " Street ", Dzongkhag: " Paro ",
		Location: domain.GeoPoint{Latitude: 28, Longitude: 90},
		Hours:    []domain.OpeningHours{{Day: 1, OpensAt: "09:00", ClosesAt: "17:00"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "New Name" || updated.Phone != "123" || updated.Address != "Street" || updated.Dzongkhag != "Paro" {
		t.Fatalf("editable details not normalized: %+v", updated)
	}
	if updated.Location.Latitude != 28 || len(updated.Hours) != 1 || updated.Hours[0].ClosesAt != "17:00" {
		t.Fatalf("location/hours not updated: %+v", updated)
	}
	if updated.Status != domain.StatusPending || updated.StatusReason != "" {
		t.Fatalf("rejected pharmacy was not resubmitted: %+v", updated)
	}
}

func TestUpdateValidatesInputAndOwnership(t *testing.T) {
	repo := memory.NewRepository()
	svc := NewService(repo, time.Now, func() string { return "ph-1" })
	_, err := svc.Register(context.Background(), RegisterInput{OwnerID: "owner-1", Name: "Name", LicenseNumber: "LIC-1", Location: domain.GeoPoint{Latitude: 27, Longitude: 89}})
	if err != nil {
		t.Fatal(err)
	}
	base := UpdateInput{Name: "Name", Location: domain.GeoPoint{Latitude: 27, Longitude: 89}}
	tests := []struct {
		name, id, owner string
		input           UpdateInput
		want            error
	}{
		{"missing id", "", "owner-1", base, domain.ErrMissingPharmacyID},
		{"missing owner", "ph-1", "", base, domain.ErrMissingOwnerID},
		{"missing name", "ph-1", "owner-1", UpdateInput{Location: base.Location}, domain.ErrMissingName},
		{"invalid location", "ph-1", "owner-1", UpdateInput{Name: "Name", Location: domain.GeoPoint{Latitude: 91}}, domain.ErrInvalidLocation},
		{"invalid hours", "ph-1", "owner-1", UpdateInput{Name: "Name", Location: base.Location, Hours: []domain.OpeningHours{{Day: 7, OpensAt: "09:00", ClosesAt: "17:00"}}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Update(context.Background(), tt.id, tt.owner, tt.input)
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.name == "invalid hours" && err == nil {
				t.Fatal("expected invalid hours error")
			}
		})
	}
	if _, err := svc.Update(context.Background(), "ph-1", "someone-else", base); !errors.Is(err, domain.ErrOwnerMismatch) {
		t.Fatalf("err = %v, want owner mismatch", err)
	}
	if _, err := svc.Update(context.Background(), "missing", "owner-1", base); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestNewID(t *testing.T) {
	id := NewID()
	if len(id) != 32 {
		t.Fatalf("id length = %d, want 32", len(id))
	}
}
