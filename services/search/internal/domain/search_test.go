package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
)

var thimphu = Point{Lat: 27.4728, Lng: 89.6390}

// Boundary values for every limit in search.proto.
func TestNewQuery(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		center    Point
		radius    int32
		maxOffers int32
		wantErr   error
		wantR     int32
		wantMax   int
	}{
		{"defaults applied", "paracetamol", thimphu, 0, 0, nil, DefaultRadiusM, DefaultMaxOffers},
		{"query trimmed", "  ibuprofen  ", thimphu, 0, 0, nil, DefaultRadiusM, DefaultMaxOffers},
		{"query 1 char", "a", thimphu, 0, 0, ErrInvalidQuery, 0, 0},
		{"query 2 chars (min)", "ab", thimphu, 0, 0, nil, DefaultRadiusM, DefaultMaxOffers},
		{"query only spaces", "    ", thimphu, 0, 0, ErrInvalidQuery, 0, 0},
		{"query 100 chars (max)", strings.Repeat("a", 100), thimphu, 0, 0, nil, DefaultRadiusM, DefaultMaxOffers},
		{"query 101 chars", strings.Repeat("a", 101), thimphu, 0, 0, ErrInvalidQuery, 0, 0},
		{"query counts characters not bytes", strings.Repeat("ཀ", 100), thimphu, 0, 0, nil, DefaultRadiusM, DefaultMaxOffers},
		{"radius 499", "ab", thimphu, 499, 0, ErrInvalidRadius, 0, 0},
		{"radius 500 (min)", "ab", thimphu, 500, 0, nil, 500, DefaultMaxOffers},
		{"radius 50000 (max)", "ab", thimphu, 50000, 0, nil, 50000, DefaultMaxOffers},
		{"radius 50001", "ab", thimphu, 50001, 0, ErrInvalidRadius, 0, 0},
		{"negative radius", "ab", thimphu, -1, 0, ErrInvalidRadius, 0, 0},
		{"max offers 1 (min)", "ab", thimphu, 0, 1, nil, DefaultRadiusM, 1},
		{"max offers 50 (max)", "ab", thimphu, 0, 50, nil, DefaultRadiusM, 50},
		{"max offers 51", "ab", thimphu, 0, 51, ErrInvalidMaxOffer, 0, 0},
		{"negative max offers", "ab", thimphu, 0, -1, ErrInvalidMaxOffer, 0, 0},
		{"latitude 90 (max)", "ab", Point{90, 0}, 0, 0, nil, DefaultRadiusM, DefaultMaxOffers},
		{"latitude 90.1", "ab", Point{90.1, 0}, 0, 0, ErrInvalidLocation, 0, 0},
		{"latitude -90.1", "ab", Point{-90.1, 0}, 0, 0, ErrInvalidLocation, 0, 0},
		{"longitude 180.1", "ab", Point{0, 180.1}, 0, 0, ErrInvalidLocation, 0, 0},
		{"longitude -180.1", "ab", Point{0, -180.1}, 0, 0, ErrInvalidLocation, 0, 0},
		{"latitude NaN", "ab", Point{math.NaN(), 0}, 0, 0, ErrInvalidLocation, 0, 0},
		{"longitude NaN", "ab", Point{0, math.NaN()}, 0, 0, ErrInvalidLocation, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := NewQuery(tc.text, tc.center, tc.radius, tc.maxOffers)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if q.RadiusM != tc.wantR || q.MaxOffers != tc.wantMax || q.Text != strings.TrimSpace(tc.text) || q.Center != tc.center {
				t.Errorf("query = %+v, want radius %d, max %d", q, tc.wantR, tc.wantMax)
			}
		})
	}
}

func TestNewSuggestQuery(t *testing.T) {
	cases := []struct {
		prefix    string
		limit     int32
		wantErr   error
		wantLimit int
	}{
		{"pa", 0, nil, DefaultSuggestions},
		{" pa ", 1, nil, 1},
		{"pa", 20, nil, 20},
		{"pa", 21, ErrInvalidLimit, 0},
		{"pa", -1, ErrInvalidLimit, 0},
		{"p", 0, ErrInvalidQuery, 0},
		{strings.Repeat("p", 101), 0, ErrInvalidQuery, 0},
	}
	for _, tc := range cases {
		q, err := NewSuggestQuery(tc.prefix, tc.limit)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("NewSuggestQuery(%q, %d) err = %v, want %v", tc.prefix, tc.limit, err, tc.wantErr)
			continue
		}
		if err == nil && (q.Limit != tc.wantLimit || q.Prefix != strings.TrimSpace(tc.prefix)) {
			t.Errorf("NewSuggestQuery(%q, %d) = %+v", tc.prefix, tc.limit, q)
		}
	}
}

func TestDisplayName(t *testing.T) {
	cases := []struct {
		p    Product
		want string
	}{
		{Product{BrandName: "Napa", Strength: "500mg", GenericName: "Paracetamol"}, "Napa 500mg (Paracetamol)"},
		{Product{BrandName: "Paracetamol", Strength: "500mg", GenericName: "paracetamol"}, "Paracetamol 500mg"},
		{Product{BrandName: "Napa", Strength: "500mg"}, "Napa 500mg"},
		{Product{BrandName: "Napa"}, "Napa"},
	}
	for _, tc := range cases {
		if got := tc.p.DisplayName(); got != tc.want {
			t.Errorf("DisplayName(%+v) = %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestHasOffers(t *testing.T) {
	if HasOffers(nil) || HasOffers([]ProductResult{{}, {}}) {
		t.Error("no offers reported as having offers")
	}
	if !HasOffers([]ProductResult{{}, {Offers: []Offer{{PharmacyID: "ph"}}}}) {
		t.Error("offers not detected")
	}
}
