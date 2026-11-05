package domain

import "testing"

func TestStatusValid(t *testing.T) {
	for _, s := range allStatuses {
		if !s.Valid() {
			t.Errorf("%s.Valid() = false, want true", s)
		}
	}
	for _, s := range []Status{"", "active", "DONE"} {
		if s.Valid() {
			t.Errorf("Status(%q).Valid() = true, want false", s)
		}
	}
}

func TestOnlyPendingAndActiveHoldsAreOpen(t *testing.T) {
	want := map[Status]bool{
		Pending:   true,
		Active:    true,
		Collected: false,
		Expired:   false,
		Cancelled: false,
		Rejected:  false,
	}
	for _, s := range allStatuses {
		if got := s.IsOpen(); got != want[s] {
			t.Errorf("%s.IsOpen() = %v, want %v", s, got, want[s])
		}
	}
}
