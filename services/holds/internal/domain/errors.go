package domain

import "errors"

// Errors a HoldRepository returns.
var (
	ErrNotFound      = errors.New("hold not found")
	ErrAlreadyExists = errors.New("hold already exists")
	// ErrConflict means the hold's status is no longer the one the caller expected,
	// because someone else changed it first.
	ErrConflict = errors.New("hold status changed since it was read")
)
