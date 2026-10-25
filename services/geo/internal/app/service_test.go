package app

import (
	"context"
	"encoding/csv"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/domain"
)

var thimphuClockTower = domain.Point{Lat: 27.4728, Lng: 89.6390}

// loadSeed upserts deploy/seed/pharmacies.csv (5 pharmacies each in Thimphu,
// Paro and Phuentsholing) and returns the dzongkhag of each pharmacy.
func loadSeed(t *testing.T, svc *Service) map[string]string {
	t.Helper()
	f, err := os.Open("../../../../deploy/seed/pharmacies.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	dzongkhag := map[string]string{}
	for _, r := range rows[1:] { // id,owner,name,license,phone,address,dzongkhag,lat,lng,status
		lat, _ := strconv.ParseFloat(r[7], 64)
		lng, _ := strconv.ParseFloat(r[8], 64)
		if err := svc.UpsertLocation(context.Background(), domain.Location{PharmacyID: r[0], Point: domain.Point{Lat: lat, Lng: lng}}); err != nil {
			t.Fatal(err)
		}
		dzongkhag[r[0]] = r[6]
	}
	return dzongkhag
}

func countBy(results []domain.Nearby, dzongkhag map[string]string) map[string]int {
	out := map[string]int{}
	for _, r := range results {
		out[dzongkhag[r.PharmacyID]]++
	}
	return out
}

func TestFindNearbyOnSeedData(t *testing.T) {
	svc := NewService(memory.NewRepository())
	dz := loadSeed(t, svc)
	ctx := context.Background()

	within2km, err := svc.FindNearby(ctx, thimphuClockTower, 2_000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := countBy(within2km, dz); got["Thimphu"] != 5 || len(got) != 1 {
		t.Errorf("2 km from Thimphu: %v, want only the 5 Thimphu pharmacies", got)
	}
	if within2km[0].DistanceMetres != 0 {
		t.Errorf("nearest = %+v, want the pharmacy at the Clock Tower (0 m)", within2km[0])
	}
	for i := 1; i < len(within2km); i++ {
		if within2km[i].DistanceMetres < within2km[i-1].DistanceMetres {
			t.Fatalf("not nearest first: %+v", within2km)
		}
	}

	within50km, err := svc.FindNearby(ctx, thimphuClockTower, 50_000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := countBy(within50km, dz); got["Thimphu"] != 5 || got["Paro"] != 5 || got["Chukha"] != 0 {
		t.Errorf("50 km from Thimphu: %v, want Thimphu 5 + Paro 5, not Phuentsholing (~73 km)", got)
	}
}

func TestUpsertMovesAndRemoveHides(t *testing.T) {
	svc := NewService(memory.NewRepository())
	ctx := context.Background()
	far := domain.Point{Lat: 26.8516, Lng: 89.3884} // Phuentsholing

	if err := svc.UpsertLocation(ctx, domain.Location{PharmacyID: "ph-1", Point: far}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.FindNearby(ctx, thimphuClockTower, 5_000, 0); len(got) != 0 {
		t.Fatalf("before move: %v, want none near Thimphu", got)
	}
	if err := svc.UpsertLocation(ctx, domain.Location{PharmacyID: "ph-1", Point: thimphuClockTower}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.FindNearby(ctx, thimphuClockTower, 5_000, 0); len(got) != 1 {
		t.Fatalf("after move: %v, want ph-1", got)
	}
	for i := 0; i < 2; i++ { // removing twice is fine
		if err := svc.RemoveLocation(ctx, "ph-1"); err != nil {
			t.Fatalf("remove #%d: %v", i+1, err)
		}
	}
	if got, _ := svc.FindNearby(ctx, thimphuClockTower, 5_000, 0); len(got) != 0 {
		t.Fatalf("after remove: %v, want none", got)
	}
}

func TestValidationErrors(t *testing.T) {
	svc := NewService(memory.NewRepository())
	ctx := context.Background()

	if _, err := svc.FindNearby(ctx, thimphuClockTower, 0, 0); !errors.Is(err, domain.ErrInvalidRadius) {
		t.Errorf("radius 0: %v", err)
	}
	if err := svc.UpsertLocation(ctx, domain.Location{PharmacyID: "ph-1", Point: domain.Point{Lat: 91}}); !errors.Is(err, domain.ErrInvalidLocation) {
		t.Errorf("bad point: %v", err)
	}
	if err := svc.RemoveLocation(ctx, ""); !errors.Is(err, domain.ErrMissingPharmacyID) {
		t.Errorf("empty id: %v", err)
	}
}

type failingRepo struct{ err error }

func (f failingRepo) Upsert(context.Context, domain.Location) error { return f.err }
func (f failingRepo) Remove(context.Context, string) error          { return f.err }
func (f failingRepo) WithinBox(context.Context, domain.Box) ([]domain.Location, error) {
	return nil, f.err
}

func TestRepositoryErrorsAreWrapped(t *testing.T) {
	dbDown := errors.New("db down")
	svc := NewService(failingRepo{dbDown})
	ctx := context.Background()

	if _, err := svc.FindNearby(ctx, thimphuClockTower, 5_000, 0); !errors.Is(err, dbDown) {
		t.Errorf("FindNearby: %v", err)
	}
	if err := svc.UpsertLocation(ctx, domain.Location{PharmacyID: "ph-1", Point: thimphuClockTower}); !errors.Is(err, dbDown) {
		t.Errorf("UpsertLocation: %v", err)
	}
	if err := svc.RemoveLocation(ctx, "ph-1"); !errors.Is(err, dbDown) {
		t.Errorf("RemoveLocation: %v", err)
	}
}
