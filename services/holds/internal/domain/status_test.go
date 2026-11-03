package domain

import (
	"errors"
	"testing"
)

var (
	allStatuses = []Status{Pending, Active, Collected, Expired, Cancelled, Rejected}
	allEvents   = []Event{StockReserved, StockUnavailable, Collect, Expire, Cancel}
)

type pair struct {
	from  Status
	event Event
}

// validMoves is the allowed list from the ticket, written out by hand.
// Every other (status, event) pair must be rejected.
var validMoves = map[pair]Status{
	{Pending, StockReserved}:    Active,
	{Pending, StockUnavailable}: Rejected,
	{Pending, Cancel}:           Cancelled,
	{Active, Collect}:           Collected,
	{Active, Expire}:            Expired,
	{Active, Cancel}:            Cancelled,
}

// Every status is tried with every event: 6 valid moves and 24 invalid ones.
func TestNextCoversEveryStatusEventPair(t *testing.T) {
	valid, invalid := 0, 0

	for _, from := range allStatuses {
		for _, event := range allEvents {
			want, isValid := validMoves[pair{from, event}]
			name := string(from) + " + " + string(event)

			t.Run(name, func(t *testing.T) {
				got, err := Next(from, event)

				if isValid {
					if err != nil || got != want {
						t.Fatalf("Next = (%q, %v), want (%q, nil)", got, err, want)
					}
					return
				}
				if !errors.Is(err, ErrInvalidTransition) || got != "" {
					t.Fatalf("Next = (%q, %v), want (\"\", ErrInvalidTransition)", got, err)
				}
			})

			if isValid {
				valid++
			} else {
				invalid++
			}
		}
	}

	if valid != 6 || invalid != 24 {
		t.Errorf("checked %d valid and %d invalid pairs, want 6 and 24", valid, invalid)
	}
}

func TestTerminalStatusesAcceptNoEvent(t *testing.T) {
	for _, from := range []Status{Collected, Expired, Cancelled, Rejected} {
		for _, event := range allEvents {
			if _, err := Next(from, event); !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("%s + %s: err = %v, want ErrInvalidTransition", from, event, err)
			}
		}
	}
}

func TestNextRejectsUnknownStatusAndEvent(t *testing.T) {
	cases := []struct {
		name  string
		from  Status
		event Event
	}{
		{"empty status", "", Collect},
		{"unknown status", "BOGUS", Cancel},
		{"unknown event", Active, "explode"},
		{"empty event", Pending, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Next(c.from, c.event); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("err = %v, want ErrInvalidTransition", err)
			}
		})
	}
}
