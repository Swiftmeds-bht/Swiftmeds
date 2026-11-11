package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

func collectedHold() Hold {
	return Hold{ID: "h-1", UserID: "u-1", PharmacyID: "ph-1", ProductID: "med-1", Collected: true}
}

func TestNewFeedbackTakesPharmacyAndProductFromTheHold(t *testing.T) {
	f, err := NewFeedback("f-1", collectedHold(), "u-1", true, 4, "  Quick and easy  ", t0.In(time.FixedZone("BTT", 6*3600)))
	if err != nil {
		t.Fatal(err)
	}
	want := Feedback{
		ID: "f-1", HoldID: "h-1", PharmacyID: "ph-1", ProductID: "med-1",
		WasInStock: true, Rating: 4, Comment: "Quick and easy", CreatedAt: t0,
	}
	if f != want {
		t.Errorf("feedback = %+v, want %+v", f, want)
	}
	if f.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt location = %v, want UTC", f.CreatedAt.Location())
	}
}

func TestNewFeedbackAcceptsNotInStockWithNoRatingAndNoComment(t *testing.T) {
	f, err := NewFeedback("f-1", collectedHold(), "u-1", false, 0, "", t0)
	if err != nil {
		t.Fatal(err)
	}
	if f.WasInStock || f.Rating != 0 || f.Comment != "" {
		t.Errorf("feedback = %+v, want not in stock, no rating, no comment", f)
	}
}

// "Feedback without a collected hold is rejected."
func TestNewFeedbackRejectsAHoldThatWasNotCollected(t *testing.T) {
	hold := collectedHold()
	hold.Collected = false

	_, err := NewFeedback("f-1", hold, "u-1", true, 5, "", t0)

	if !errors.Is(err, ErrHoldNotCollected) {
		t.Errorf("err = %v, want ErrHoldNotCollected", err)
	}
}

func TestNewFeedbackRejectsAnotherPatientsHold(t *testing.T) {
	_, err := NewFeedback("f-1", collectedHold(), "u-2", true, 5, "", t0)
	if !errors.Is(err, ErrNotHoldOwner) {
		t.Errorf("err = %v, want ErrNotHoldOwner", err)
	}
}

func TestNewFeedbackChecksTheOwnerBeforeWhetherItWasCollected(t *testing.T) {
	hold := collectedHold()
	hold.Collected = false

	_, err := NewFeedback("f-1", hold, "u-2", true, 5, "", t0)

	if !errors.Is(err, ErrNotHoldOwner) {
		t.Errorf("err = %v, want ErrNotHoldOwner so a stranger learns nothing about the hold's state", err)
	}
}

func TestNewFeedbackRejectsMissingIDs(t *testing.T) {
	noHold := collectedHold()
	noHold.ID = ""
	if _, err := NewFeedback("f-1", noHold, "u-1", true, 0, "", t0); !errors.Is(err, ErrMissingHoldID) {
		t.Errorf("err = %v, want ErrMissingHoldID", err)
	}
	if _, err := NewFeedback("f-1", collectedHold(), "", true, 0, "", t0); !errors.Is(err, ErrMissingUserID) {
		t.Errorf("err = %v, want ErrMissingUserID", err)
	}
}

func TestValidateAnswer(t *testing.T) {
	cases := []struct {
		name    string
		rating  int
		comment string
		want    error
	}{
		{"no rating", 0, "", nil},
		{"lowest rating", 1, "", nil},
		{"highest rating", 5, "", nil},
		{"rating below zero", -1, "", ErrInvalidRating},
		{"rating above five", 6, "", ErrInvalidRating},
		{"comment at the limit", 3, strings.Repeat("a", MaxCommentLength), nil},
		{"comment one over the limit", 3, strings.Repeat("a", MaxCommentLength+1), ErrCommentTooLong},
		{"limit counts characters, not bytes", 3, strings.Repeat("འ", MaxCommentLength), nil},
		{"one more character than the limit", 3, strings.Repeat("འ", MaxCommentLength+1), ErrCommentTooLong},
		{"spaces around the comment do not count", 3, "  " + strings.Repeat("a", MaxCommentLength) + "  ", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateAnswer(tc.rating, tc.comment); !errors.Is(err, tc.want) {
				t.Errorf("ValidateAnswer(%d, %d chars) = %v, want %v", tc.rating, len(tc.comment), err, tc.want)
			}
		})
	}
}
