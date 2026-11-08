package domain

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

func TestNewDeadlineKeepsTheHoldAndTheTimeInUTC(t *testing.T) {
	local := time.FixedZone("BTT", 6*60*60) // Bhutan time, UTC+6
	expires := time.Date(2026, 11, 6, 16, 0, 0, 0, local)

	d, err := NewDeadline("h-1", expires)

	if err != nil {
		t.Fatal(err)
	}
	if d.HoldID != "h-1" || !d.ExpiresAt.Equal(t0) || d.ExpiresAt.Location() != time.UTC {
		t.Errorf("got %+v, want h-1 at %v in UTC", d, t0)
	}
}

func TestNewDeadlineRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name    string
		holdID  string
		expires time.Time
		wantErr error
	}{
		{"no hold id", "", t0, ErrMissingHoldID},
		{"only spaces as hold id", "  ", t0, ErrMissingHoldID},
		{"no expiry time", "h-1", time.Time{}, ErrMissingExpiry},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewDeadline(c.holdID, c.expires)

			if !errors.Is(err, c.wantErr) || got != (Deadline{}) {
				t.Fatalf("got (%+v, %v), want (empty deadline, %v)", got, err, c.wantErr)
			}
		})
	}
}

// The ticket: "Boundary test: a hold expiring exactly now."
func TestDeadlineIsDueExactlyWhenItExpires(t *testing.T) {
	d, _ := NewDeadline("h-1", t0)

	cases := []struct {
		name string
		asOf time.Time
		want bool
	}{
		{"a day before", t0.Add(-24 * time.Hour), false},
		{"one second before", t0.Add(-time.Second), false},
		{"one nanosecond before", t0.Add(-time.Nanosecond), false},
		{"exactly at the deadline", t0, true},
		{"one nanosecond after", t0.Add(time.Nanosecond), true},
		{"one second after", t0.Add(time.Second), true},
		{"a day after", t0.Add(24 * time.Hour), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := d.Due(c.asOf); got != c.want {
				t.Errorf("Due(%v) = %v, want %v", c.asOf, got, c.want)
			}
		})
	}
}

func TestDueComparesInstantsNotTimeZones(t *testing.T) {
	d, _ := NewDeadline("h-1", t0)
	sameInstantInBhutan := t0.In(time.FixedZone("BTT", 6*60*60))

	if !d.Due(sameInstantInBhutan) {
		t.Error("the same instant written in another time zone is not due")
	}
}
