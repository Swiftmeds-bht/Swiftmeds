package domain

import "errors"

// Errors a HoldRepository returns.
var (
	ErrNotFound      = errors.New("hold not found")
	ErrAlreadyExists = errors.New("hold already exists")
	// ErrConflict means the hold's status is no longer the one the caller expected,
	// because someone else changed it first.
	ErrConflict = errors.New("hold status changed since it was read")
	// ErrLimitReached means the patient already has the most open holds
	// (PENDING or ACTIVE) that are allowed at the same time.
	ErrLimitReached = errors.New("too many open holds for this patient")
	// ErrPickupCodeTaken means another ACTIVE hold at the same pharmacy already
	// uses this pickup code.
	ErrPickupCodeTaken = errors.New("pickup code already in use at this pharmacy")
)

// Errors for a rejected request. The gRPC adapter maps each one to a status code.
var (
	// ErrMissingPickupCode means the pickup code was empty.
	ErrMissingPickupCode = errors.New("pickup_code is required")
	// ErrWrongPickupCode means the pickup code does not match the hold's code.
	ErrWrongPickupCode = errors.New("pickup code does not match")
	// ErrWrongPharmacy means the hold was made at a different pharmacy.
	ErrWrongPharmacy = errors.New("hold belongs to a different pharmacy")
	// ErrNotHoldOwner means the hold belongs to a different patient.
	ErrNotHoldOwner = errors.New("hold belongs to a different patient")
	// ErrHoldExpired means the hold's time has run out, so it cannot be collected.
	ErrHoldExpired = errors.New("hold has expired")
	// ErrNotYetExpired means ExpireHold was called before expires_at.
	ErrNotYetExpired = errors.New("hold has not reached its expiry time")
	// ErrInvalidListFilter means a list request must name a patient or a pharmacy
	// (exactly one), and a known status if it names one.
	ErrInvalidListFilter = errors.New("set exactly one of user_id and pharmacy_id, and a valid status")
	// ErrInvalidPageToken means the page token was not made by this service.
	ErrInvalidPageToken = errors.New("page_token is not valid")
)

// Errors from the saga's calls to other systems.
var (
	// ErrInventoryUnavailable means a call to Inventory failed, so the hold was
	// not changed. The caller may try again: every Inventory call is safe to repeat.
	ErrInventoryUnavailable = errors.New("stock service is not available")
	// ErrRequestNotSent means CreateHold could not publish hold.requested. The
	// hold is rejected instead of being left PENDING for ever, and the patient may try again.
	ErrRequestNotSent = errors.New("could not send the stock reservation request")
)
