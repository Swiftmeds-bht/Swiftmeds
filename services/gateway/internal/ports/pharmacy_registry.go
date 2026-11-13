package ports

import (
	"context"

	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
)

type PharmacyRegistryClient interface {
	GetPharmacy(context.Context, *pharmacyregistryv1.GetPharmacyRequest) (*pharmacyregistryv1.GetPharmacyResponse, error)
}
