package clients

import (
	"context"

	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	searchv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/search/v1"
	substitutionv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/substitution/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
)

var _ ports.Searcher = (*SearchClient)(nil)
var _ ports.NearbyPharmacyFinder = (*GeoClient)(nil)
var _ ports.SubstituteFinder = (*SubstitutionClient)(nil)

type SearchClient struct{ client searchv1.SearchServiceClient }

func NewSearchClient(client searchv1.SearchServiceClient) *SearchClient {
	return &SearchClient{client: client}
}

func (c *SearchClient) SearchMedicine(ctx context.Context, req *searchv1.SearchMedicineRequest) (*searchv1.SearchMedicineResponse, error) {
	return c.client.SearchMedicine(ctx, req)
}

type GeoClient struct{ client geov1.GeoServiceClient }

func NewGeoClient(client geov1.GeoServiceClient) *GeoClient {
	return &GeoClient{client: client}
}

func (c *GeoClient) FindNearbyPharmacies(ctx context.Context, req *geov1.FindNearbyPharmaciesRequest) (*geov1.FindNearbyPharmaciesResponse, error) {
	return c.client.FindNearbyPharmacies(ctx, req)
}

type SubstitutionClient struct {
	client substitutionv1.SubstitutionServiceClient
}

func NewSubstitutionClient(client substitutionv1.SubstitutionServiceClient) *SubstitutionClient {
	return &SubstitutionClient{client: client}
}

func (c *SubstitutionClient) GetSubstitutes(ctx context.Context, req *substitutionv1.GetSubstitutesRequest) (*substitutionv1.GetSubstitutesResponse, error) {
	return c.client.GetSubstitutes(ctx, req)
}
