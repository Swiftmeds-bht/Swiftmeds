package ports

import (
	"context"

	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	searchv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/search/v1"
	substitutionv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/substitution/v1"
)

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
