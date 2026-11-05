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
		Location: &commonv1.GeoPoint{Latitude: 27.43, Longitude: 89.41},
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.GetPharmacy().GetStatus() != registryv1.VerificationStatus_VERIFICATION_STATUS_PENDING {
		t.Fatalf("new pharmacy status = %v, want pending", registered.GetPharmacy().GetStatus())
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
