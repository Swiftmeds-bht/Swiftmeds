// Package clients holds the outbound gRPC adapters Watchlist calls.
package clients

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/ports"
)

// geoMaxLimit is the most pharmacies Geo returns for one query (geo.proto, MaxLimit).
const geoMaxLimit = 100

var _ ports.PharmacyLocator = Geo{}

// Geo answers "is this pharmacy near the subscriber?" with FindNearbyPharmacies:
// the frozen Geo contract has no lookup of one pharmacy's location, so we ask for
// the pharmacies near the subscriber and look for this one.
//
// Limitation: only the nearest 100 pharmacies are checked. A pharmacy farther away
// than 100 others inside the radius is treated as not near; that is a dense city
// and a large radius, far beyond Bhutan's pharmacy counts.
type Geo struct{ C geov1.GeoServiceClient }

func NewGeo(cc grpc.ClientConnInterface) Geo { return Geo{geov1.NewGeoServiceClient(cc)} }

func (g Geo) WithinRadius(ctx context.Context, pharmacyID string, center domain.Point, radiusM int32) (bool, error) {
	resp, err := g.C.FindNearbyPharmacies(ctx, &geov1.FindNearbyPharmaciesRequest{
		Location: &commonv1.GeoPoint{Latitude: center.Lat, Longitude: center.Lng},
		RadiusM:  radiusM,
		Limit:    geoMaxLimit,
	})
	if err != nil {
		return false, fmt.Errorf("geo FindNearbyPharmacies: %w", err)
	}
	for _, p := range resp.GetPharmacies() {
		if p.GetPharmacyId() == pharmacyID {
			return true, nil
		}
	}
	return false, nil
}
