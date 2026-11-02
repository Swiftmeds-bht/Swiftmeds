package domain

import (
	"errors"
	"testing"
	"time"
)

// With the default thresholds, HIGH is under 6 hours and MEDIUM is under 24 hours.
// An age exactly on a threshold belongs to the lower level.
func TestConfidenceForBoundaryValues(t *testing.T) {
	th := DefaultThresholds()
	const ns = time.Nanosecond

	cases := []struct {
		name string
		age  time.Duration
		want Confidence
	}{
		{"just confirmed", 0, High},
		{"confirmed in the future (clock skew)", -time.Minute, High},
		{"just under the high edge", 6*time.Hour - ns, High},
		{"exactly on the high edge", 6 * time.Hour, Medium},
		{"just over the high edge", 6*time.Hour + ns, Medium},
		{"just under the medium edge", 24*time.Hour - ns, Medium},
		{"exactly on the medium edge", 24 * time.Hour, Low},
		{"just over the medium edge", 24*time.Hour + ns, Low},
		{"very old", 30 * 24 * time.Hour, Low},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := th.ConfidenceFor(c.age); got != c.want {
				t.Errorf("ConfidenceFor(%v) = %s, want %s", c.age, got, c.want)
			}
		})
	}
}

func TestConfidenceForUsesCustomThresholds(t *testing.T) {
	th, err := NewThresholds(time.Hour, 2*time.Hour)
	if err != nil {
		t.Fatalf("NewThresholds: %v", err)
	}

	cases := []struct {
		age  time.Duration
		want Confidence
	}{
		{time.Hour - time.Nanosecond, High},
		{time.Hour, Medium},
		{2*time.Hour - time.Nanosecond, Medium},
		{2 * time.Hour, Low},
	}
	for _, c := range cases {
		if got := th.ConfidenceFor(c.age); got != c.want {
			t.Errorf("ConfidenceFor(%v) = %s, want %s", c.age, got, c.want)
		}
	}
}

func TestNewThresholdsRejectsBadValues(t *testing.T) {
	cases := []struct {
		name         string
		high, medium time.Duration
	}{
		{"zero high", 0, time.Hour},
		{"negative high", -time.Hour, time.Hour},
		{"medium equal to high", time.Hour, time.Hour},
		{"medium shorter than high", 2 * time.Hour, time.Hour},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewThresholds(c.high, c.medium); !errors.Is(err, ErrInvalidThresholds) {
				t.Fatalf("err = %v, want ErrInvalidThresholds", err)
			}
		})
	}
}

func TestScore(t *testing.T) {
	cases := map[Confidence]float64{High: 1.0, Medium: 0.6, Low: 0.2}
	for confidence, want := range cases {
		if got := confidence.Score(); got != want {
			t.Errorf("%s.Score() = %v, want %v", confidence, got, want)
		}
	}
	if !(High.Score() > Medium.Score() && Medium.Score() > Low.Score()) {
		t.Error("scores must go down from HIGH to LOW")
	}
}

func TestNewConfirmationValidates(t *testing.T) {
	at := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name                  string
		pharmacyID, productID string
		confirmedAt           time.Time
		wantErr               error
	}{
		{"valid", "ph-1", "med-1", at, nil},
		{"missing pharmacy", "", "med-1", at, ErrMissingPharmacyID},
		{"missing product", "ph-1", "", at, ErrMissingProductID},
		{"missing time", "ph-1", "med-1", time.Time{}, ErrMissingConfirmedAt},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewConfirmation(c.pharmacyID, c.productID, c.confirmedAt)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if err == nil && (got.PharmacyID != "ph-1" || got.ProductID != "med-1" || !got.ConfirmedAt.Equal(at)) {
				t.Errorf("got %+v, fields were not kept", got)
			}
		})
	}
}

func TestConfirmationConfidenceUsesAgeAtNow(t *testing.T) {
	confirmedAt := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	c, err := NewConfirmation("ph-1", "med-1", confirmedAt)
	if err != nil {
		t.Fatalf("NewConfirmation: %v", err)
	}
	th := DefaultThresholds()

	cases := []struct {
		now  time.Time
		want Confidence
	}{
		{confirmedAt.Add(5 * time.Hour), High},
		{confirmedAt.Add(6 * time.Hour), Medium},
		{confirmedAt.Add(24 * time.Hour), Low},
	}
	for _, tc := range cases {
		if got := c.Confidence(th, tc.now); got != tc.want {
			t.Errorf("at %v: got %s, want %s", tc.now, got, tc.want)
		}
	}
}

// Decay recomputes from the same inputs, so running it twice gives the same answer.
func TestConfirmationConfidenceIsRepeatable(t *testing.T) {
	confirmedAt := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	c, _ := NewConfirmation("ph-1", "med-1", confirmedAt)
	th := DefaultThresholds()
	now := confirmedAt.Add(10 * time.Hour)

	first := c.Confidence(th, now)
	second := c.Confidence(th, now)
	if first != second {
		t.Error("same inputs gave different answers")
	}
}
