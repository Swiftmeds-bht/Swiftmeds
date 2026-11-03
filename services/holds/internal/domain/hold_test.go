package domain

import (
	"errors"
	"testing"
	"time"
)

var (
	t0      = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	holdFor = 2 * time.Hour
)

// holdIn returns a hold that is in the given status.
func holdIn(status Status) Hold {
	h, _ := NewHold("h-1", "u-1", "ph-1", "med-1", 2, t0)
	h.Status = status
	return h
}

func TestNewHoldValidates(t *testing.T) {
	cases := []struct {
		name                              string
		id, userID, pharmacyID, productID string
		quantity                          int
		wantErr                           error
	}{
		{"valid", "h-1", "u-1", "ph-1", "med-1", 2, nil},
		{"missing hold id", "", "u-1", "ph-1", "med-1", 2, ErrMissingHoldID},
		{"missing user id", "h-1", "", "ph-1", "med-1", 2, ErrMissingUserID},
		{"missing pharmacy id", "h-1", "u-1", "", "med-1", 2, ErrMissingPharmacyID},
		{"missing product id", "h-1", "u-1", "ph-1", "", 2, ErrMissingProductID},
		{"quantity below the minimum", "h-1", "u-1", "ph-1", "med-1", 0, ErrInvalidQuantity},
		{"negative quantity", "h-1", "u-1", "ph-1", "med-1", -1, ErrInvalidQuantity},
		{"quantity at the minimum", "h-1", "u-1", "ph-1", "med-1", 1, nil},
		{"quantity at the maximum", "h-1", "u-1", "ph-1", "med-1", 5, nil},
		{"quantity above the maximum", "h-1", "u-1", "ph-1", "med-1", 6, ErrInvalidQuantity},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewHold(c.id, c.userID, c.pharmacyID, c.productID, c.quantity, t0)

			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
		})
	}
}

func TestNewHoldStartsPendingWithNoExpiryTime(t *testing.T) {
	h, err := NewHold("h-1", "u-1", "ph-1", "med-1", 3, t0)

	if err != nil {
		t.Fatal(err)
	}
	if h.ID != "h-1" || h.UserID != "u-1" || h.PharmacyID != "ph-1" || h.ProductID != "med-1" || h.Quantity != 3 {
		t.Errorf("fields were not kept: %+v", h)
	}
	if h.Status != Pending {
		t.Errorf("status = %s, want PENDING", h.Status)
	}
	if !h.CreatedAt.Equal(t0) || !h.StatusChangedAt.Equal(t0) {
		t.Errorf("created_at = %v, status_changed_at = %v, want both %v", h.CreatedAt, h.StatusChangedAt, t0)
	}
	if !h.ExpiresAt.IsZero() {
		t.Errorf("expires_at = %v, want zero until the hold is ACTIVE", h.ExpiresAt)
	}
	if h.StatusReason != "" {
		t.Errorf("status_reason = %q, want empty", h.StatusReason)
	}
}

// The hold methods must agree with the transition table for every status and event.
func TestHoldMethodsFollowTheTransitionTable(t *testing.T) {
	now := t0.Add(10 * time.Minute)
	apply := map[Event]func(Hold) (Hold, error){
		StockReserved:    func(h Hold) (Hold, error) { return h.Reserve(now, holdFor) },
		StockUnavailable: func(h Hold) (Hold, error) { return h.Reject(now, "no stock") },
		Collect:          func(h Hold) (Hold, error) { return h.Collect(now) },
		Expire:           func(h Hold) (Hold, error) { return h.Expire(now) },
		Cancel:           func(h Hold) (Hold, error) { return h.Cancel(now, "changed my mind") },
	}

	for _, from := range allStatuses {
		for _, event := range allEvents {
			want, isValid := validMoves[pair{from, event}]

			t.Run(string(from)+" + "+string(event), func(t *testing.T) {
				before := holdIn(from)

				got, err := apply[event](before)

				if !isValid {
					if !errors.Is(err, ErrInvalidTransition) || got != (Hold{}) {
						t.Fatalf("got (%+v, %v), want (empty hold, ErrInvalidTransition)", got, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				if got.Status != want || !got.StatusChangedAt.Equal(now) {
					t.Errorf("status = %s changed at %v, want %s changed at %v", got.Status, got.StatusChangedAt, want, now)
				}
				if before.Status != from {
					t.Errorf("the original hold changed to %s, want it to stay %s", before.Status, from)
				}
			})
		}
	}
}

func TestExpiryTimeIsSetWhenTheHoldBecomesActive(t *testing.T) {
	h, _ := NewHold("h-1", "u-1", "ph-1", "med-1", 1, t0)
	activatedAt := t0.Add(30 * time.Second) // Inventory answers a little after the hold was created

	active, err := h.Reserve(activatedAt, holdFor)

	if err != nil {
		t.Fatal(err)
	}
	if want := activatedAt.Add(holdFor); !active.ExpiresAt.Equal(want) {
		t.Errorf("expires_at = %v, want %v (activation time + duration, not creation time + duration)", active.ExpiresAt, want)
	}
	if !h.ExpiresAt.IsZero() {
		t.Error("the original PENDING hold got an expiry time")
	}
}

func TestExpiryTimeStaysAfterLaterTransitions(t *testing.T) {
	h, _ := NewHold("h-1", "u-1", "ph-1", "med-1", 1, t0)
	active, _ := h.Reserve(t0, holdFor)

	collected, err := active.Collect(t0.Add(time.Hour))

	if err != nil {
		t.Fatal(err)
	}
	if !collected.ExpiresAt.Equal(active.ExpiresAt) {
		t.Errorf("expires_at = %v, want %v to be kept", collected.ExpiresAt, active.ExpiresAt)
	}
}

func TestReserveRejectsAnInvalidDuration(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Hour} {
		h := holdIn(Pending)

		got, err := h.Reserve(t0, d)

		if !errors.Is(err, ErrInvalidDuration) || got != (Hold{}) {
			t.Errorf("duration %v: got (%+v, %v), want (empty hold, ErrInvalidDuration)", d, got, err)
		}
	}
}

func TestHoldThatWasNeverActiveHasNoExpiryTime(t *testing.T) {
	rejected, _ := holdIn(Pending).Reject(t0, "no stock")
	cancelled, _ := holdIn(Pending).Cancel(t0, "changed my mind")

	if !rejected.ExpiresAt.IsZero() || !cancelled.ExpiresAt.IsZero() {
		t.Errorf("expires_at: rejected %v, cancelled %v, want both zero", rejected.ExpiresAt, cancelled.ExpiresAt)
	}
}

func TestRejectAndCancelRecordTheReason(t *testing.T) {
	rejected, _ := holdIn(Pending).Reject(t0, "no stock")
	cancelled, _ := holdIn(Active).Cancel(t0, "changed my mind")
	collected, _ := holdIn(Active).Collect(t0)
	expired, _ := holdIn(Active).Expire(t0)

	if rejected.StatusReason != "no stock" || cancelled.StatusReason != "changed my mind" {
		t.Errorf("reasons: rejected %q, cancelled %q", rejected.StatusReason, cancelled.StatusReason)
	}
	if collected.StatusReason != "" || expired.StatusReason != "" {
		t.Errorf("reasons: collected %q, expired %q, want both empty", collected.StatusReason, expired.StatusReason)
	}
}
