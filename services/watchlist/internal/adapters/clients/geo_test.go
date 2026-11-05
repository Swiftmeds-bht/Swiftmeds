package clients

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"

	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
)

type fakeGeoClient struct {
	geov1.GeoServiceClient
	got  *geov1.FindNearbyPharmaciesRequest
	resp *geov1.FindNearbyPharmaciesResponse
	err  error
}

func (f *fakeGeoClient) FindNearbyPharmacies(_ context.Context, req *geov1.FindNearbyPharmaciesRequest, _ ...grpc.CallOption) (*geov1.FindNearbyPharmaciesResponse, error) {
	f.got = req
	return f.resp, f.err
}

var thimphu = domain.Point{Lat: 27.4728, Lng: 89.639}

func TestWithinRadius(t *testing.T) {
	fake := &fakeGeoClient{resp: &geov1.FindNearbyPharmaciesResponse{Pharmacies: []*geov1.NearbyPharmacy{
		{PharmacyId: "ph-1", DistanceM: 300},
		{PharmacyId: "ph-2", DistanceM: 900},
	}}}
	geo := Geo{C: fake}

	near, err := geo.WithinRadius(context.Background(), "ph-2", thimphu, 2000)
	if err != nil || !near {
		t.Fatalf("ph-2: near=%v err=%v, want true", near, err)
	}
	if fake.got.GetRadiusM() != 2000 || fake.got.GetLimit() != geoMaxLimit ||
		fake.got.GetLocation().GetLatitude() != thimphu.Lat || fake.got.GetLocation().GetLongitude() != thimphu.Lng {
		t.Errorf("request = %+v", fake.got)
	}

	if near, _ := geo.WithinRadius(context.Background(), "ph-9", thimphu, 2000); near {
		t.Error("ph-9 is not in Geo's answer but was reported near")
	}
}

func TestWithinRadiusPassesOnErrors(t *testing.T) {
	geo := Geo{C: &fakeGeoClient{err: errors.New("unavailable")}}
	if _, err := geo.WithinRadius(context.Background(), "ph-1", thimphu, 2000); err == nil {
		t.Fatal("want an error when Geo fails")
	}
}

func TestNewGeo(t *testing.T) {
	if NewGeo(nil).C == nil {
		t.Fatal("NewGeo returned no client")
	}
}
