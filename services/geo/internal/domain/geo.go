// Package domain holds Geo's rules: coordinates, great-circle distance and the
// "nearby" query. It imports nothing outside the standard library.
package domain

import (
	"cmp"
	"errors"
	"math"
	"slices"
)

const (
	// EarthRadiusMetres is the IUGG mean Earth radius.
	EarthRadiusMetres = 6_371_008.8

	MinRadiusMetres = 1
	// MaxRadiusMetres matches the largest radius Search accepts (search.proto).
	MaxRadiusMetres = 50_000

	DefaultLimit = 20
	MaxLimit     = 100
)

var (
	ErrInvalidLocation   = errors.New("latitude must be -90..90 and longitude -180..180")
	ErrInvalidRadius     = errors.New("radius_m must be between 1 and 50000")
	ErrInvalidLimit      = errors.New("limit must be between 0 and 100")
	ErrMissingPharmacyID = errors.New("pharmacy_id is required")
)

// Point is a WGS84 coordinate in degrees.
type Point struct {
	Lat, Lng float64
}

func (p Point) Validate() error {
	if math.IsNaN(p.Lat) || math.IsNaN(p.Lng) || p.Lat < -90 || p.Lat > 90 || p.Lng < -180 || p.Lng > 180 {
		return ErrInvalidLocation
	}
	return nil
}

// Location is where a verified pharmacy is.
type Location struct {
	PharmacyID string
	Point      Point
}

func (l Location) Validate() error {
	if l.PharmacyID == "" {
		return ErrMissingPharmacyID
	}
	return l.Point.Validate()
}

// Nearby is one result: a pharmacy and its distance in whole metres.
type Nearby struct {
	PharmacyID     string
	DistanceMetres int32
}

// DistanceMetres is the great-circle (Haversine) distance between a and b.
func DistanceMetres(a, b Point) float64 {
	lat1, lat2 := radians(a.Lat), radians(b.Lat)
	dLat := lat2 - lat1
	dLng := radians(b.Lng - a.Lng)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * EarthRadiusMetres * math.Asin(math.Min(1, math.Sqrt(h)))
}

// Box is a latitude/longitude rectangle used to pre-filter candidates in SQL.
type Box struct {
	MinLat, MaxLat, MinLng, MaxLng float64
}

// BoundingBox returns a box that contains every point within radius of
// center. It is deliberately a little too large: the exact Haversine check in
// Find removes the corners. Near a pole or the 180° meridian it widens to all
// longitudes, which is still correct, just less selective.
func BoundingBox(center Point, radiusMetres float64) Box {
	dLat := degrees(radiusMetres / EarthRadiusMetres)
	box := Box{
		MinLat: math.Max(-90, center.Lat-dLat),
		MaxLat: math.Min(90, center.Lat+dLat),
		MinLng: -180,
		MaxLng: 180,
	}
	if box.MinLat > -90 && box.MaxLat < 90 {
		// Longitude degrees shrink with latitude; use the widest row of the box.
		maxAbsLat := math.Max(math.Abs(box.MinLat), math.Abs(box.MaxLat))
		dLng := degrees(radiusMetres / (EarthRadiusMetres * math.Cos(radians(maxAbsLat))))
		if center.Lng-dLng >= -180 && center.Lng+dLng <= 180 {
			box.MinLng, box.MaxLng = center.Lng-dLng, center.Lng+dLng
		}
	}
	return box
}

// Query is a validated "nearby" request.
type Query struct {
	Center       Point
	RadiusMetres int32
	Limit        int
}

// NewQuery validates a request. limit 0 means DefaultLimit.
func NewQuery(center Point, radiusMetres, limit int32) (Query, error) {
	if err := center.Validate(); err != nil {
		return Query{}, err
	}
	if radiusMetres < MinRadiusMetres || radiusMetres > MaxRadiusMetres {
		return Query{}, ErrInvalidRadius
	}
	if limit < 0 || limit > MaxLimit {
		return Query{}, ErrInvalidLimit
	}
	if limit == 0 {
		limit = DefaultLimit
	}
	return Query{Center: center, RadiusMetres: radiusMetres, Limit: int(limit)}, nil
}

// edgeTolerance absorbs floating-point error so a pharmacy exactly on the
// radius counts as inside (1 µm).
const edgeTolerance = 1e-6

// Find keeps the candidates within the radius, nearest first (ties by
// pharmacy ID, so the order is deterministic), and applies the limit.
func (q Query) Find(candidates []Location) []Nearby {
	type hit struct {
		id string
		d  float64
	}
	hits := make([]hit, 0, len(candidates))
	for _, c := range candidates {
		if d := DistanceMetres(q.Center, c.Point); d <= float64(q.RadiusMetres)+edgeTolerance {
			hits = append(hits, hit{c.PharmacyID, d})
		}
	}
	slices.SortFunc(hits, func(a, b hit) int {
		return cmp.Or(cmp.Compare(a.d, b.d), cmp.Compare(a.id, b.id))
	})
	if len(hits) > q.Limit {
		hits = hits[:q.Limit]
	}
	out := make([]Nearby, 0, len(hits))
	for _, h := range hits {
		out = append(out, Nearby{PharmacyID: h.id, DistanceMetres: int32(math.Round(h.d))})
	}
	return out
}

func radians(deg float64) float64 { return deg * math.Pi / 180 }
func degrees(rad float64) float64 { return rad * 180 / math.Pi }
