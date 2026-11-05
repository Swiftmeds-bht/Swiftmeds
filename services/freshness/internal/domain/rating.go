package domain

import "errors"

// ErrNotFound means no confirmation has been recorded for the stock row.
var ErrNotFound = errors.New("no confirmation recorded for this stock row")

// NewKey validates and builds a Key.
func NewKey(pharmacyID, productID string) (Key, error) {
	switch {
	case pharmacyID == "":
		return Key{}, ErrMissingPharmacyID
	case productID == "":
		return Key{}, ErrMissingProductID
	}
	return Key{PharmacyID: pharmacyID, ProductID: productID}, nil
}

// Rating is a confirmation together with its stored confidence.
type Rating struct {
	Confirmation
	Confidence Confidence
}
