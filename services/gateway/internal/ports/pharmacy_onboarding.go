package ports

import (
	"context"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
)

// PharmacyOnboarding is what the pharmacy sign-up and admin review routes need
// from Pharmacy Registry (SCRUM-141).
type PharmacyOnboarding interface {
	RegisterPharmacy(context.Context, *pharmacyregistryv1.RegisterPharmacyRequest) (*pharmacyregistryv1.RegisterPharmacyResponse, error)
	GetPharmacy(context.Context, *pharmacyregistryv1.GetPharmacyRequest) (*pharmacyregistryv1.GetPharmacyResponse, error)
	ListPharmacies(context.Context, *pharmacyregistryv1.ListPharmaciesRequest) (*pharmacyregistryv1.ListPharmaciesResponse, error)
	SetVerificationStatus(context.Context, *pharmacyregistryv1.SetVerificationStatusRequest) (*pharmacyregistryv1.SetVerificationStatusResponse, error)
	UpdatePharmacy(context.Context, *pharmacyregistryv1.UpdatePharmacyRequest) (*pharmacyregistryv1.UpdatePharmacyResponse, error)
}

// PharmacyLinker makes an account the staff account of the pharmacy it
// registered (Auth.LinkPharmacy).
type PharmacyLinker interface {
	LinkPharmacy(context.Context, *authv1.LinkPharmacyRequest) (*authv1.LinkPharmacyResponse, error)
}

// PharmacyLocations keeps Geo's map in step with verification: verified
// pharmacies are on it, suspended ones are not.
type PharmacyLocations interface {
	UpsertPharmacyLocation(context.Context, *geov1.UpsertPharmacyLocationRequest) (*geov1.UpsertPharmacyLocationResponse, error)
	RemovePharmacyLocation(context.Context, *geov1.RemovePharmacyLocationRequest) (*geov1.RemovePharmacyLocationResponse, error)
}
