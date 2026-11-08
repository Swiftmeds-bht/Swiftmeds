package domain

import "errors"

// Errors for a rejected request. The gRPC adapter maps each one to a status code.
var (
	ErrMissingHoldID     = errors.New("hold_id is required")
	ErrMissingUserID     = errors.New("user_id is required")
	ErrMissingPharmacyID = errors.New("pharmacy_id is required")
	ErrInvalidRating     = errors.New("rating must be 0 (not given) or 1 to 5")
	ErrCommentTooLong    = errors.New("comment must be at most 500 characters")
	ErrInvalidPageToken  = errors.New("page_token is not valid")

	// ErrHoldNotFound means Holds has no hold with this ID.
	ErrHoldNotFound = errors.New("hold not found")
	// ErrNotHoldOwner means the hold belongs to a different patient.
	ErrNotHoldOwner = errors.New("hold belongs to a different patient")
	// ErrHoldNotCollected means the hold is not COLLECTED: feedback is only for
	// medicine the patient actually picked up.
	ErrHoldNotCollected = errors.New("feedback is only allowed for a collected hold")
)

// Errors the outside world can cause.
var (
	// ErrAlreadySubmitted means this hold already has its one feedback.
	ErrAlreadySubmitted = errors.New("feedback for this hold was already submitted")
	// ErrHoldsUnavailable means the Holds service could not answer in time.
	ErrHoldsUnavailable = errors.New("holds service is unavailable")
	// ErrPublishFailed means the feedback.submitted event could not be sent. The
	// feedback was not stored, so the caller can safely try again.
	ErrPublishFailed = errors.New("could not publish feedback.submitted; nothing was saved")
)
