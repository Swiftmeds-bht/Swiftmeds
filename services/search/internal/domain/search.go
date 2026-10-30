// Package domain holds Search's request rules and result types. Search owns no
// data: everything here is validation and shapes. Standard library only.
package domain

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits from proto/search/v1/search.proto.
const (
	DefaultRadiusM = 5_000
	MinRadiusM     = 500
	MaxRadiusM     = 50_000

	DefaultMaxOffers = 10
	MaxMaxOffers     = 50

	MinQueryLen = 2
	MaxQueryLen = 100

	DefaultSuggestions = 8
	MaxSuggestions     = 20
)

var (
	ErrInvalidQuery    = errors.New("query must be 2 to 100 characters")
	ErrInvalidLocation = errors.New("location must have latitude -90..90 and longitude -180..180")
	ErrInvalidRadius   = errors.New("radius_m must be 0 (default 5000) or 500..50000")
	ErrInvalidMaxOffer = errors.New("max_offers must be 0 (default 10) or 1..50")
	ErrInvalidLimit    = errors.New("limit must be 0 (default 8) or 1..20")
)

type Point struct {
	Lat, Lng float64
}

func (p Point) valid() bool {
	return !math.IsNaN(p.Lat) && !math.IsNaN(p.Lng) && p.Lat >= -90 && p.Lat <= 90 && p.Lng >= -180 && p.Lng <= 180
}

// Query is a validated search request.
type Query struct {
	Text      string
	Center    Point
	RadiusM   int32
	MaxOffers int
}

// NewQuery trims and validates a request and applies the defaults (radius 0 ->
// 5000 m, max offers 0 -> 10).
func NewQuery(text string, center Point, radiusM, maxOffers int32) (Query, error) {
	text = strings.TrimSpace(text)
	if n := utf8.RuneCountInString(text); n < MinQueryLen || n > MaxQueryLen {
		return Query{}, ErrInvalidQuery
	}
	if !center.valid() {
		return Query{}, ErrInvalidLocation
	}
	if radiusM == 0 {
		radiusM = DefaultRadiusM
	}
	if radiusM < MinRadiusM || radiusM > MaxRadiusM {
		return Query{}, ErrInvalidRadius
	}
	if maxOffers == 0 {
		maxOffers = DefaultMaxOffers
	}
	if maxOffers < 1 || maxOffers > MaxMaxOffers {
		return Query{}, ErrInvalidMaxOffer
	}
	return Query{Text: text, Center: center, RadiusM: radiusM, MaxOffers: int(maxOffers)}, nil
}

// SuggestQuery is a validated autocomplete request.
type SuggestQuery struct {
	Prefix string
	Limit  int
}

func NewSuggestQuery(prefix string, limit int32) (SuggestQuery, error) {
	prefix = strings.TrimSpace(prefix)
	if n := utf8.RuneCountInString(prefix); n < MinQueryLen || n > MaxQueryLen {
		return SuggestQuery{}, ErrInvalidQuery
	}
	if limit == 0 {
		limit = DefaultSuggestions
	}
	if limit < 1 || limit > MaxSuggestions {
		return SuggestQuery{}, ErrInvalidLimit
	}
	return SuggestQuery{Prefix: prefix, Limit: int(limit)}, nil
}

// Key identifies one product at one pharmacy.
type Key struct {
	PharmacyID, ProductID string
}

// Product is a catalogue entry that matched the query.
type Product struct {
	ID, BrandName, GenericName, Strength string
}

// DisplayName is how a product is shown in suggestions, e.g. "Napa 500mg (Paracetamol)".
func (p Product) DisplayName() string {
	name := strings.TrimSpace(p.BrandName + " " + p.Strength)
	if p.GenericName != "" && !strings.EqualFold(p.GenericName, p.BrandName) {
		name += " (" + p.GenericName + ")"
	}
	return name
}

// Offer is one pharmacy that has the product, with everything the result shows.
type Offer struct {
	PharmacyID    string
	PharmacyName  string
	Address       string
	Location      *Point // nil when Pharmacy Registry did not answer
	DistanceM     int32
	Quantity      int32
	PriceChhertum int64 // 0 = no price listed
	Freshness     float64
	Trust         float64
	RankScore     float64
	ConfirmedAt   time.Time
}

// ProductResult is one product and its offers, best first.
type ProductResult struct {
	Product
	Offers []Offer
}

// Result is the answer to one search.
type Result struct {
	SearchID    string
	Results     []ProductResult
	Substitutes []ProductResult
	// Degraded lists optional services that did not answer in time, so the
	// results are partial (e.g. "freshness"). Empty when everything answered.
	Degraded []string
}

// HasOffers reports whether any result has at least one in-stock pharmacy.
func HasOffers(results []ProductResult) bool {
	for _, r := range results {
		if len(r.Offers) > 0 {
			return true
		}
	}
	return false
}
