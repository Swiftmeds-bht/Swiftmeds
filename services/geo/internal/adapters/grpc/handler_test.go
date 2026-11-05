package grpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	geov1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/geo/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/app"
)

var thimphu = &commonv1.GeoPoint{Latitude: 27.4728, Longitude: 89.6390}

func newHandler() *Handler { return NewHandler(app.NewService(memory.NewRepository())) }

func TestUpsertFindRemove(t *testing.T) {
	h := newHandler()
	ctx := context.Background()

	if _, err := h.UpsertPharmacyLocation(ctx, &geov1.UpsertPharmacyLocationRequest{PharmacyId: "ph-1", Location: thimphu}); err != nil {
		t.Fatal(err)
	}
	resp, err := h.FindNearbyPharmacies(ctx, &geov1.FindNearbyPharmaciesRequest{Location: thimphu, RadiusM: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetPharmacies()) != 1 || resp.GetPharmacies()[0].GetPharmacyId() != "ph-1" || resp.GetPharmacies()[0].GetDistanceM() != 0 {
		t.Fatalf("FindNearbyPharmacies = %v", resp.GetPharmacies())
	}

	if _, err := h.RemovePharmacyLocation(ctx, &geov1.RemovePharmacyLocationRequest{PharmacyId: "ph-1"}); err != nil {
		t.Fatal(err)
	}
	resp, err = h.FindNearbyPharmacies(ctx, &geov1.FindNearbyPharmaciesRequest{Location: thimphu, RadiusM: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetPharmacies()) != 0 {
		t.Fatalf("after remove: %v, want none", resp.GetPharmacies())
	}
}

func TestInvalidRequestsAreInvalidArgument(t *testing.T) {
	h := newHandler()
	ctx := context.Background()

	cases := map[string]error{}
	_, cases["missing location"] = h.FindNearbyPharmacies(ctx, &geov1.FindNearbyPharmaciesRequest{RadiusM: 1000})
	_, cases["radius 0"] = h.FindNearbyPharmacies(ctx, &geov1.FindNearbyPharmaciesRequest{Location: thimphu})
	_, cases["radius too large"] = h.FindNearbyPharmacies(ctx, &geov1.FindNearbyPharmaciesRequest{Location: thimphu, RadiusM: 50_001})
	_, cases["limit too large"] = h.FindNearbyPharmacies(ctx, &geov1.FindNearbyPharmaciesRequest{Location: thimphu, RadiusM: 1000, Limit: 101})
	_, cases["upsert without location"] = h.UpsertPharmacyLocation(ctx, &geov1.UpsertPharmacyLocationRequest{PharmacyId: "ph-1"})
	_, cases["upsert bad latitude"] = h.UpsertPharmacyLocation(ctx, &geov1.UpsertPharmacyLocationRequest{
		PharmacyId: "ph-1", Location: &commonv1.GeoPoint{Latitude: 95},
	})
	_, cases["upsert without id"] = h.UpsertPharmacyLocation(ctx, &geov1.UpsertPharmacyLocationRequest{Location: thimphu})
	_, cases["remove without id"] = h.RemovePharmacyLocation(ctx, &geov1.RemovePharmacyLocationRequest{})

	for name, err := range cases {
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: code = %v, want InvalidArgument", name, status.Code(err))
		}
	}
}

func TestUnexpectedErrorsAreInternalWithoutDetails(t *testing.T) {
	err := toStatus(errors.New("pq: connection refused"))
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
		t.Errorf("toStatus = %v, want a generic Internal", err)
	}
}
