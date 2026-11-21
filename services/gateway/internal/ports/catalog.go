package ports

import (
	"context"

	catalogv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/catalog/v1"
)

// ProductLookup reads one medicine from the Catalog (SCRUM-138).
type ProductLookup interface {
	GetProduct(context.Context, *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error)
}
