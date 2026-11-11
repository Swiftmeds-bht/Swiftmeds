package domain

import "errors"

var (
	ErrMissingPharmacyID = errors.New("pharmacy ID is required")
	ErrMissingUploaderID = errors.New("uploaded-by user ID is required")
	ErrMissingJobID      = errors.New("job ID is required")
	ErrJobNotFound       = errors.New("sync job not found")
	ErrInvalidPageToken  = errors.New("page token is invalid")
)
