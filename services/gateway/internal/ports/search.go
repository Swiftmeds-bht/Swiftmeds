package ports

import (
	"context"
	"sync"

	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	searchv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/search/v1"
	substitutionv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/substitution/v1"
)

type searchMetadataKey struct{}

type searchMetadata struct {
	mu       sync.Mutex
	degraded string
}

// WithSearchMetadata lets the Search adapter return response metadata alongside
// its protobuf response without coupling the HTTP handler to gRPC call options.
func WithSearchMetadata(ctx context.Context) (context.Context, func() string) {
	metadata := &searchMetadata{}
	ctx = context.WithValue(ctx, searchMetadataKey{}, metadata)
	get := func() string {
		metadata.mu.Lock()
		defer metadata.mu.Unlock()
		return metadata.degraded
	}
	return ctx, get
}

func SetSearchDegraded(ctx context.Context, value string) {
	metadata, _ := ctx.Value(searchMetadataKey{}).(*searchMetadata)
	if metadata != nil {
		metadata.mu.Lock()
		metadata.degraded = value
		metadata.mu.Unlock()
	}
}

type Searcher interface {
	SearchMedicine(context.Context, *searchv1.SearchMedicineRequest) (*searchv1.SearchMedicineResponse, error)
}

// Suggester offers medicine names while the patient types (Search Autocomplete).
type Suggester interface {
	Autocomplete(context.Context, *searchv1.AutocompleteRequest) (*searchv1.AutocompleteResponse, error)
}

type NearbyPharmacyFinder interface {
	FindNearbyPharmacies(context.Context, *geov1.FindNearbyPharmaciesRequest) (*geov1.FindNearbyPharmaciesResponse, error)
}

type SubstituteFinder interface {
	GetSubstitutes(context.Context, *substitutionv1.GetSubstitutesRequest) (*substitutionv1.GetSubstitutesResponse, error)
}
