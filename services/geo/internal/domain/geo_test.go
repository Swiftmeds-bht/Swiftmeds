package domain

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

var (
	thimphu       = Point{Lat: 27.4728, Lng: 89.6390} // Clock Tower Square
	phuentsholing = Point{Lat: 26.8516, Lng: 89.3884}
)

// northOf returns the point exactly metres due north of p (along a meridian
// the great-circle distance is just the latitude difference).
func northOf(p Point, metres float64) Point {
	return Point{Lat: p.Lat + degrees(metres/EarthRadiusMetres), Lng: p.Lng}
}

func TestDistanceThimphuToPhuentsholingWithin1Percent(t *testing.T) {
	// Reference: 73.39 km, from an independent equirectangular calculation on
	// the same coordinates (commonly quoted as about 73 km in a straight line).
	const want = 73_390.0
	got := DistanceMetres(thimphu, phuentsholing)
	if math.Abs(got-want)/want > 0.01 {
		t.Fatalf("distance = %.0f m, want within 1%% of %.0f m", got, want)
	}
}

func TestDistanceProperties(t *testing.T) {
	if d := DistanceMetres(thimphu, thimphu); d != 0 {
		t.Errorf("distance to itself = %v, want 0", d)
	}
	if a, b := DistanceMetres(thimphu, phuentsholing), DistanceMetres(phuentsholing, thimphu); math.Abs(a-b) > 1e-6 {
		t.Errorf("distance is not symmetric: %v vs %v", a, b)
	}
	if d := DistanceMetres(thimphu, northOf(thimphu, 1000)); math.Abs(d-1000) > 1e-6 {
		t.Errorf("1 km due north = %v m, want 1000", d)
	}
	// Antipodal points are half the circumference apart.
	if d := DistanceMetres(Point{0, 0}, Point{0, 180}); math.Abs(d-math.Pi*EarthRadiusMetres) > 1 {
		t.Errorf("antipodal distance = %v, want %v", d, math.Pi*EarthRadiusMetres)
	}
}

func TestPointValidate(t *testing.T) {
	cases := []struct {
		p    Point
		want error
	}{
		{thimphu, nil},
		{Point{90, 180}, nil},
		{Point{-90, -180}, nil},
		{Point{90.0001, 0}, ErrInvalidLocation},
		{Point{-90.0001, 0}, ErrInvalidLocation},
		{Point{0, 180.0001}, ErrInvalidLocation},
		{Point{0, -180.0001}, ErrInvalidLocation},
		{Point{math.NaN(), 0}, ErrInvalidLocation},
	}
	for _, tc := range cases {
		if err := tc.p.Validate(); !errors.Is(err, tc.want) {
			t.Errorf("Validate(%v) = %v, want %v", tc.p, err, tc.want)
		}
	}
}

func TestLocationValidate(t *testing.T) {
	if err := (Location{PharmacyID: "", Point: thimphu}).Validate(); !errors.Is(err, ErrMissingPharmacyID) {
		t.Errorf("missing ID: %v", err)
	}
	if err := (Location{PharmacyID: "ph-1", Point: Point{100, 0}}).Validate(); !errors.Is(err, ErrInvalidLocation) {
		t.Errorf("bad point: %v", err)
	}
	if err := (Location{PharmacyID: "ph-1", Point: thimphu}).Validate(); err != nil {
		t.Errorf("valid location: %v", err)
	}
}

// Radius and limit boundaries.
func TestNewQuery(t *testing.T) {
	cases := []struct {
		name      string
		radius    int32
		limit     int32
		wantErr   error
		wantLimit int
	}{
		{"radius 0 is rejected", 0, 0, ErrInvalidRadius, 0},
		{"radius 1 (min)", 1, 0, nil, DefaultLimit},
		{"radius 50000 (max)", MaxRadiusMetres, 0, nil, DefaultLimit},
		{"radius 50001 is rejected", MaxRadiusMetres + 1, 0, ErrInvalidRadius, 0},
		{"negative radius", -5, 0, ErrInvalidRadius, 0},
		{"limit 1", 5000, 1, nil, 1},
		{"limit 100 (max)", 5000, MaxLimit, nil, MaxLimit},
		{"limit 101 is rejected", 5000, MaxLimit + 1, ErrInvalidLimit, 0},
		{"negative limit", 5000, -1, ErrInvalidLimit, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := NewQuery(thimphu, tc.radius, tc.limit)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err == nil && q.Limit != tc.wantLimit {
				t.Errorf("limit = %d, want %d", q.Limit, tc.wantLimit)
			}
		})
	}
	if _, err := NewQuery(Point{91, 0}, 5000, 0); !errors.Is(err, ErrInvalidLocation) {
		t.Errorf("invalid centre: %v", err)
	}
}

func mustQuery(t *testing.T, radius, limit int32) Query {
	t.Helper()
	q, err := NewQuery(thimphu, radius, limit)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestFindEdgeIsInclusive(t *testing.T) {
	q := mustQuery(t, 1000, 0)
	got := q.Find([]Location{
		{"on-edge", northOf(thimphu, 1000)},
		{"just-outside", northOf(thimphu, 1000.01)},
	})
	want := []Nearby{{PharmacyID: "on-edge", DistanceMetres: 1000}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Find = %+v, want %+v", got, want)
	}
}

func TestFindSortsNearestFirstWithDeterministicTies(t *testing.T) {
	q := mustQuery(t, 5000, 0)
	got := q.Find([]Location{
		{"far", northOf(thimphu, 3000)},
		{"tie-b", northOf(thimphu, 500)},
		{"near", northOf(thimphu, 100)},
		{"tie-a", Point{Lat: thimphu.Lat - degrees(500/EarthRadiusMetres), Lng: thimphu.Lng}}, // 500 m south
	})
	var ids []string
	for _, n := range got {
		ids = append(ids, n.PharmacyID)
	}
	if want := []string{"near", "tie-a", "tie-b", "far"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("order = %v, want %v", ids, want)
	}
}

func TestFindAppliesLimitAfterSorting(t *testing.T) {
	q := mustQuery(t, 5000, 2)
	got := q.Find([]Location{{"c", northOf(thimphu, 300)}, {"a", northOf(thimphu, 100)}, {"b", northOf(thimphu, 200)}})
	if len(got) != 2 || got[0].PharmacyID != "a" || got[1].PharmacyID != "b" {
		t.Errorf("Find = %+v, want the 2 nearest (a, b)", got)
	}
}

func TestFindNothingNearby(t *testing.T) {
	if got := mustQuery(t, 5000, 0).Find([]Location{{"phuentsholing", phuentsholing}}); len(got) != 0 {
		t.Errorf("Find = %+v, want none within 5 km of Thimphu", got)
	}
	if got := mustQuery(t, 5000, 0).Find(nil); len(got) != 0 {
		t.Errorf("Find(nil) = %+v, want none", got)
	}
}

func TestBoundingBoxContainsEveryPointInRadius(t *testing.T) {
	const r = 50_000
	box := BoundingBox(thimphu, r)
	// Points at the radius in 8 directions must all fall inside the box.
	for _, bearing := range []float64{0, 45, 90, 135, 180, 225, 270, 315} {
		p := destination(thimphu, r, bearing)
		if p.Lat < box.MinLat || p.Lat > box.MaxLat || p.Lng < box.MinLng || p.Lng > box.MaxLng {
			t.Errorf("bearing %v°: %v outside box %+v", bearing, p, box)
		}
	}
	// And the box must actually filter (not span the whole globe here).
	if box.MaxLng-box.MinLng > 2 || box.MaxLat-box.MinLat > 2 {
		t.Errorf("box %+v is unexpectedly large", box)
	}
}

func TestBoundingBoxNearPoleAndDateline(t *testing.T) {
	if box := BoundingBox(Point{Lat: 89.9, Lng: 0}, 50_000); box.MinLng != -180 || box.MaxLng != 180 || box.MaxLat != 90 {
		t.Errorf("near pole: %+v, want all longitudes up to 90°N", box)
	}
	if box := BoundingBox(Point{Lat: 0, Lng: 179.9}, 50_000); box.MinLng != -180 || box.MaxLng != 180 {
		t.Errorf("across 180°: %+v, want all longitudes", box)
	}
}

// destination is the point metres away from p on the given bearing.
func destination(p Point, metres, bearingDeg float64) Point {
	δ := metres / EarthRadiusMetres
	θ := radians(bearingDeg)
	φ1, λ1 := radians(p.Lat), radians(p.Lng)
	φ2 := math.Asin(math.Sin(φ1)*math.Cos(δ) + math.Cos(φ1)*math.Sin(δ)*math.Cos(θ))
	λ2 := λ1 + math.Atan2(math.Sin(θ)*math.Sin(δ)*math.Cos(φ1), math.Cos(δ)-math.Sin(φ1)*math.Sin(φ2))
	return Point{Lat: degrees(φ2), Lng: degrees(λ2)}
}
