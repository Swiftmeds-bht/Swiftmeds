package domain

import "errors"

var (
	ErrMissingProductID  = errors.New("product ID is required")
	ErrMissingMoleculeID = errors.New("product molecule ID is required")
)
