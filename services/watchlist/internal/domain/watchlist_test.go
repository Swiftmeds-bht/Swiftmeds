package domain

import (
	"errors"
	"testing"
	"time"
)

var (
	now     = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	thimphu = Point{Lat: 27.4728, Lng: 89.639}
)

func TestNewSubscriptionValid(t *testing.T) {
	s, err := NewSubscription("sub-1", " user-1 ", " pr-1 ", thimphu, 5000, now)
	if err != nil {
		t.Fatalf("NewSubscription: %v", err)
	}
	want := Subscription{ID: "sub-1", UserID: "user-1", ProductID: "pr-1", Location: thimphu, RadiusM: 5000, CreatedAt: now}
	if s != want {
		t.Errorf("got %+v, want %+v", s, want)
	}
	if s.Notified() {
		t.Error("a new subscription is already notified")
	}
}

func TestNewSubscriptionDefaultsRadius(t *testing.T) {
	s, err := NewSubscription("sub-1", "user-1", "pr-1", thimphu, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if s.RadiusM != DefaultRadiusM {
		t.Errorf("radius = %d, want default %d", s.RadiusM, DefaultRadiusM)
	}
}

func TestNewSubscriptionRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		user    string
		product string
		at      Point
		radius  int32
		want    error
	}{
		{"missing user", " ", "pr-1", thimphu, 5000, ErrMissingUserID},
		{"missing product", "user-1", "", thimphu, 5000, ErrMissingProductID},
		{"latitude too high", "user-1", "pr-1", Point{Lat: 90.0001, Lng: 0}, 5000, ErrInvalidLocation},
		{"latitude too low", "user-1", "pr-1", Point{Lat: -90.0001, Lng: 0}, 5000, ErrInvalidLocation},
		{"longitude too high", "user-1", "pr-1", Point{Lat: 0, Lng: 180.0001}, 5000, ErrInvalidLocation},
		{"longitude too low", "user-1", "pr-1", Point{Lat: 0, Lng: -180.0001}, 5000, ErrInvalidLocation},
		{"negative radius", "user-1", "pr-1", thimphu, -1, ErrInvalidRadius},
		{"radius too large", "user-1", "pr-1", thimphu, MaxRadiusM + 1, ErrInvalidRadius},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSubscription("sub-1", tt.user, tt.product, tt.at, tt.radius, now)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewSubscriptionAcceptsBoundaries(t *testing.T) {
	for _, tt := range []struct {
		at     Point
		radius int32
	}{
		{Point{Lat: 90, Lng: 180}, MinRadiusM},
		{Point{Lat: -90, Lng: -180}, MaxRadiusM},
	} {
		if _, err := NewSubscription("sub-1", "user-1", "pr-1", tt.at, tt.radius, now); err != nil {
			t.Errorf("NewSubscription(%+v, %d): %v", tt.at, tt.radius, err)
		}
	}
}

func TestStockIncreaseValidate(t *testing.T) {
	ok := StockIncrease{PharmacyID: "ph-1", ProductID: "pr-1", NewAvailable: 3, OccurredAt: now}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid event: %v", err)
	}
	if err := (StockIncrease{ProductID: "pr-1", NewAvailable: 3}).Validate(); !errors.Is(err, ErrMissingPharmacyID) {
		t.Errorf("missing pharmacy: err = %v", err)
	}
	if err := (StockIncrease{PharmacyID: "ph-1", NewAvailable: 3}).Validate(); !errors.Is(err, ErrMissingProductID) {
		t.Errorf("missing product: err = %v", err)
	}
}

func TestStockIncreaseInStock(t *testing.T) {
	for _, tt := range []struct {
		available int32
		want      bool
	}{{-1, false}, {0, false}, {1, true}, {12, true}} {
		if got := (StockIncrease{NewAvailable: tt.available}).InStock(); got != tt.want {
			t.Errorf("InStock(%d) = %v, want %v", tt.available, got, tt.want)
		}
	}
}

func TestNotified(t *testing.T) {
	s := Subscription{}
	if s.Notified() {
		t.Error("zero NotifiedAt reported as notified")
	}
	s.NotifiedAt = now
	if !s.Notified() {
		t.Error("set NotifiedAt not reported as notified")
	}
}
