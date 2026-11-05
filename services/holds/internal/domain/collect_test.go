package domain

import (
	"errors"
	"testing"
	"time"
)

func TestReserveKeepsThePickupCode(t *testing.T) {
	pending := holdIn(Pending)

	active, err := pending.Reserve(t0, holdFor, testCode)

	if err != nil {
		t.Fatal(err)
	}
	if active.PickupCode != testCode {
		t.Errorf("pickup code = %q, want %q", active.PickupCode, testCode)
	}
	if pending.PickupCode != "" {
		t.Error("the original PENDING hold got a pickup code")
	}
}

func TestReserveNeedsAPickupCode(t *testing.T) {
	got, err := holdIn(Pending).Reserve(t0, holdFor, "")

	if !errors.Is(err, ErrMissingPickupCode) || got != (Hold{}) {
		t.Fatalf("got (%+v, %v), want (empty hold, ErrMissingPickupCode)", got, err)
	}
}

func TestReserveChecksTheStatusBeforeTheCode(t *testing.T) {
	_, err := holdIn(Active).Reserve(t0, holdFor, "")

	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestCollect(t *testing.T) {
	expiresAt := t0.Add(holdFor)
	cases := []struct {
		name       string
		now        time.Time
		pharmacyID string
		code       string
		wantErr    error
	}{
		{"right code", t0.Add(time.Hour), "ph-1", testCode, nil},
		{"right code in lower case with a dash", t0.Add(time.Hour), "ph-1", "abc-234", nil},
		{"right code one second before the hold expires", expiresAt.Add(-time.Second), "ph-1", testCode, nil},
		{"wrong code", t0.Add(time.Hour), "ph-1", "ZZZ999", ErrWrongPickupCode},
		{"code that is almost right", t0.Add(time.Hour), "ph-1", "ABC235", ErrWrongPickupCode},
		{"no code", t0.Add(time.Hour), "ph-1", "", ErrMissingPickupCode},
		{"only spaces as the code", t0.Add(time.Hour), "ph-1", "   ", ErrMissingPickupCode},
		{"no pharmacy", t0.Add(time.Hour), "", testCode, ErrMissingPharmacyID},
		{"another pharmacy", t0.Add(time.Hour), "ph-2", testCode, ErrWrongPharmacy},
		{"right code exactly when the hold expires", expiresAt, "ph-1", testCode, ErrHoldExpired},
		{"right code after the hold expired", expiresAt.Add(time.Minute), "ph-1", testCode, ErrHoldExpired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := holdIn(Active)

			got, err := before.Collect(c.now, c.pharmacyID, c.code)

			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if c.wantErr != nil {
				if got != (Hold{}) {
					t.Errorf("got %+v, want an empty hold", got)
				}
				return
			}
			if got.Status != Collected || !got.StatusChangedAt.Equal(c.now) {
				t.Errorf("status = %s changed at %v, want COLLECTED changed at %v", got.Status, got.StatusChangedAt, c.now)
			}
			if before.Status != Active {
				t.Errorf("the original hold changed to %s, want it to stay ACTIVE", before.Status)
			}
		})
	}
}

func TestCollectingTwiceFails(t *testing.T) {
	first, err := holdIn(Active).Collect(t0.Add(time.Hour), "ph-1", testCode)
	if err != nil {
		t.Fatal(err)
	}

	second, err := first.Collect(t0.Add(time.Hour), "ph-1", testCode)

	if !errors.Is(err, ErrInvalidTransition) || second != (Hold{}) {
		t.Fatalf("got (%+v, %v), want (empty hold, ErrInvalidTransition)", second, err)
	}
}

func TestCollectChecksTheStatusBeforeTheCode(t *testing.T) {
	// A hold that is not ACTIVE says "invalid transition" even when the code is wrong.
	for _, status := range []Status{Pending, Collected, Expired, Cancelled, Rejected} {
		_, err := holdIn(status).Collect(t0, "ph-1", "ZZZ999")

		if !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("%s: err = %v, want ErrInvalidTransition", status, err)
		}
	}
}
