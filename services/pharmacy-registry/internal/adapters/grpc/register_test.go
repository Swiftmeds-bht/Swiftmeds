package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	registryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
)

func registration(owner, license string) *registryv1.RegisterPharmacyRequest {
	return &registryv1.RegisterPharmacyRequest{
		OwnerUserId: owner, Name: "Thimphu Care", LicenseNumber: license,
		Phone: "+975 2 334 455", Address: "Norzin Lam, Thimphu", Dzongkhag: "Thimphu",
		Location: &commonv1.GeoPoint{Latitude: 27.47, Longitude: 89.64},
	}
}

// SCRUM-141: phone and address reach the administrator who verifies the pharmacy.
func TestRegisterKeepsPhoneAndAddress(t *testing.T) {
	h := newHandler()
	resp, err := h.RegisterPharmacy(context.Background(), registration("owner-1", "LIC-9"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.GetPharmacy(context.Background(), &registryv1.GetPharmacyRequest{PharmacyId: resp.GetPharmacy().GetPharmacyId()})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPharmacy().GetPhone() != "+975 2 334 455" || got.GetPharmacy().GetAddress() != "Norzin Lam, Thimphu" {
		t.Fatalf("phone=%q address=%q", got.GetPharmacy().GetPhone(), got.GetPharmacy().GetAddress())
	}
}

// A licence number already used by another owner is ALREADY_EXISTS (409 at the gateway), not INTERNAL.
func TestRegisterDuplicateLicenceIsAlreadyExists(t *testing.T) {
	h := newHandler()
	ctx := context.Background()
	if _, err := h.RegisterPharmacy(ctx, registration("owner-1", "LIC-9")); err != nil {
		t.Fatal(err)
	}
	_, err := h.RegisterPharmacy(ctx, registration("owner-2", "LIC-9"))
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("code = %v, want AlreadyExists (err %v)", status.Code(err), err)
	}
}
