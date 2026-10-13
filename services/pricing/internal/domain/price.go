// Package domain holds Pricing's entities and business rules.
// It imports nothing outside the standard library.
package domain

import (
	"errors"
	"time"
)

var (
	ErrMissingPharmacyID = errors.New("pharmacy_id is required")
	ErrMissingProductID  = errors.New("product_id is required")
	ErrNonPositivePrice  = errors.New("price must be greater than zero")
)

// Key identifies one product at one pharmacy.
type Key struct {
	PharmacyID string
	ProductID  string
}

// Price is a pharmacy's price for one pack of a product, in chhertum (100 = Nu. 1).
type Price struct {
	Key
	AmountChhertum int64
	UpdatedAt      time.Time
}

// NewPrice validates and builds a Price.
func NewPrice(pharmacyID, productID string, amountChhertum int64, now time.Time) (Price, error) {
	switch {
	case pharmacyID == "":
		return Price{}, ErrMissingPharmacyID
	case productID == "":
		return Price{}, ErrMissingProductID
	case amountChhertum <= 0:
		return Price{}, ErrNonPositivePrice
	}
	return Price{
		Key:            Key{PharmacyID: pharmacyID, ProductID: productID},
		AmountChhertum: amountChhertum,
		UpdatedAt:      now,
	}, nil
}
