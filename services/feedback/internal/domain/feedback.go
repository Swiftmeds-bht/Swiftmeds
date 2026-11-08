// Package domain holds the Feedback rules. It uses only the standard library.
package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxRating is the best rating. 0 means the patient gave none.
	MaxRating = 5
	// MaxCommentLength is the most characters a comment may have (feedback.proto).
	MaxCommentLength = 500
)

// Hold is what Feedback needs to know about a hold. It is read from the Holds
// service and never stored here.
type Hold struct {
	ID         string
	UserID     string
	PharmacyID string
	ProductID  string
	// Collected is true when the hold's status is COLLECTED.
	Collected bool
}

// Feedback is a patient's answer to "was the medicine there?" for one collected hold.
type Feedback struct {
	ID         string
	HoldID     string
	PharmacyID string
	ProductID  string
	WasInStock bool
	// Rating is 1 to 5, or 0 if not given.
	Rating    int
	Comment   string
	CreatedAt time.Time
}

// ValidateAnswer checks the parts of a feedback the patient writes.
func ValidateAnswer(rating int, comment string) error {
	if rating < 0 || rating > MaxRating {
		return ErrInvalidRating
	}
	if utf8.RuneCountInString(strings.TrimSpace(comment)) > MaxCommentLength {
		return ErrCommentTooLong
	}
	return nil
}

// NewFeedback makes the feedback of userID for hold, or says why it is refused.
// The pharmacy and product come from the hold, never from the caller, so a
// patient cannot leave feedback about a pharmacy where they did not collect.
//
// The checks run in this order: the request is complete and well formed, the
// hold belongs to this patient, and only then whether it was collected.
func NewFeedback(id string, hold Hold, userID string, wasInStock bool, rating int, comment string, now time.Time) (Feedback, error) {
	if hold.ID == "" {
		return Feedback{}, ErrMissingHoldID
	}
	if userID == "" {
		return Feedback{}, ErrMissingUserID
	}
	if err := ValidateAnswer(rating, comment); err != nil {
		return Feedback{}, err
	}
	if hold.UserID != userID {
		return Feedback{}, ErrNotHoldOwner
	}
	if !hold.Collected {
		return Feedback{}, ErrHoldNotCollected
	}
	return Feedback{
		ID:         id,
		HoldID:     hold.ID,
		PharmacyID: hold.PharmacyID,
		ProductID:  hold.ProductID,
		WasInStock: wasInStock,
		Rating:     rating,
		Comment:    strings.TrimSpace(comment),
		CreatedAt:  now.UTC(),
	}, nil
}
