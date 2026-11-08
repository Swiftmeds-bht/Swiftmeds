package domain

import "testing"

func TestOnlyAnEndedHoldIsFinal(t *testing.T) {
	want := map[HoldStatus]bool{
		HoldPending:   false,
		HoldActive:    false,
		HoldCollected: true,
		HoldExpired:   true,
		HoldCancelled: true,
		HoldRejected:  true,
		"":            false,
		"SOMETHING":   false,
	}
	for status, final := range want {
		if got := status.Final(); got != final {
			t.Errorf("%q.Final() = %v, want %v", status, got, final)
		}
	}
}
