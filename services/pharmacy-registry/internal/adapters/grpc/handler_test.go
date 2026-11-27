package grpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	registryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/app"
)

func newHandler() *Handler {
	next := 0
	return NewHandler(app.NewService(memory.NewRepository(), func() time.Time {
		return time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	}, func() string {
		next++
		return "ph-" + string(rune('0'+next))
	}))
}

func TestRegisterVerifyAndVerifiedOnlyList(t *testing.T) {
	h := newHandler()
	ctx := context.Background()
	registered, err := h.RegisterPharmacy(ctx, &registryv1.RegisterPharmacyRequest{
		OwnerUserId: "owner-1", Name: "Paro Pharmacy", LicenseNumber: "LIC-1",
		Dzongkhag: "  Paro  ",
		Location:  &commonv1.GeoPoint{Latitude: 27.43, Longitude: 89.41},
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.GetPharmacy().GetStatus() != registryv1.VerificationStatus_VERIFICATION_STATUS_PENDING {
		t.Fatalf("new pharmacy status = %v, want pending", registered.GetPharmacy().GetStatus())
	}
	if registered.GetPharmacy().GetDzongkhag() != "Paro" {
		t.Fatalf("dzongkhag = %q, want %q", registered.GetPharmacy().GetDzongkhag(), "Paro")
	}
	if _, err := h.SetVerificationStatus(ctx, &registryv1.SetVerificationStatusRequest{
		PharmacyId: registered.GetPharmacy().GetPharmacyId(),
		Status:     registryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED,
	}); err != nil {
		t.Fatal(err)
	}
	listed, err := h.ListPharmacies(ctx, &registryv1.ListPharmaciesRequest{
		Status: registryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetPharmacies()) != 1 {
		t.Fatalf("verified list has %d pharmacies, want 1", len(listed.GetPharmacies()))
	}
}

func TestInvalidVerificationStatusIsInvalidArgument(t *testing.T) {
	h := newHandler()
	created, err := h.RegisterPharmacy(context.Background(), &registryv1.RegisterPharmacyRequest{
		OwnerUserId: "owner-1", Name: "Name", LicenseNumber: "LIC-1",
		Location: &commonv1.GeoPoint{Latitude: 27, Longitude: 89},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.SetVerificationStatus(context.Background(), &registryv1.SetVerificationStatusRequest{
		PharmacyId: created.GetPharmacy().GetPharmacyId(),
		Status:     registryv1.VerificationStatus_VERIFICATION_STATUS_PENDING,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestReinstateAndUpdatePharmacy(t *testing.T) {
	h := newHandler()
	ctx := context.Background()
	created, err := h.RegisterPharmacy(ctx, &registryv1.RegisterPharmacyRequest{OwnerUserId: "owner-1", Name: "Old", LicenseNumber: "LIC-U", Location: &commonv1.GeoPoint{Latitude: 27, Longitude: 89}})
	if err != nil {
		t.Fatal(err)
	}
	id := created.GetPharmacy().GetPharmacyId()
	if _, err = h.SetVerificationStatus(ctx, &registryv1.SetVerificationStatusRequest{PharmacyId: id, Status: registryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.SetVerificationStatus(ctx, &registryv1.SetVerificationStatusRequest{PharmacyId: id, Status: registryv1.VerificationStatus_VERIFICATION_STATUS_SUSPENDED, Reason: "temporary"}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.SetVerificationStatus(ctx, &registryv1.SetVerificationStatusRequest{PharmacyId: id, Status: registryv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED}); err != nil {
		t.Fatalf("reinstate: %v", err)
	}
	_, err = h.UpdatePharmacy(ctx, &registryv1.UpdatePharmacyRequest{PharmacyId: id, OwnerUserId: "wrong", Name: "New", Location: &commonv1.GeoPoint{Latitude: 28, Longitude: 90}})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("wrong owner code=%v err=%v", status.Code(err), err)
	}
	updated, err := h.UpdatePharmacy(ctx, &registryv1.UpdatePharmacyRequest{PharmacyId: id, OwnerUserId: "owner-1", Name: "New", Phone: "123", Address: "Road", Dzongkhag: "Paro", Location: &commonv1.GeoPoint{Latitude: 28, Longitude: 90}, Hours: []*registryv1.OpeningHours{{Day: 1, OpensAt: "09:00", ClosesAt: "17:00"}}})
	if err != nil {
		t.Fatal(err)
	}
	if p := updated.GetPharmacy(); p.GetName() != "New" || p.GetLocation().GetLatitude() != 28 || len(p.GetHours()) != 1 {
		t.Fatalf("updated pharmacy=%v", p)
	}
}

func TestRejectedOwnerUpdateResubmits(t *testing.T) {
	h := newHandler()
	ctx := context.Background()
	created, err := h.RegisterPharmacy(ctx, &registryv1.RegisterPharmacyRequest{OwnerUserId: "owner-1", Name: "Old", LicenseNumber: "LIC-R", Location: &commonv1.GeoPoint{Latitude: 27, Longitude: 89}})
	if err != nil {
		t.Fatal(err)
	}
	id := created.GetPharmacy().GetPharmacyId()
	if _, err = h.SetVerificationStatus(ctx, &registryv1.SetVerificationStatusRequest{PharmacyId: id, Status: registryv1.VerificationStatus_VERIFICATION_STATUS_REJECTED, Reason: "correct details"}); err != nil {
		t.Fatal(err)
	}
	updated, err := h.UpdatePharmacy(ctx, &registryv1.UpdatePharmacyRequest{PharmacyId: id, OwnerUserId: "owner-1", Name: "Fixed", Location: &commonv1.GeoPoint{Latitude: 27, Longitude: 89}})
	if err != nil || updated.GetPharmacy().GetStatus() != registryv1.VerificationStatus_VERIFICATION_STATUS_PENDING {
		t.Fatalf("resubmission=%v err=%v", updated, err)
	}
}
