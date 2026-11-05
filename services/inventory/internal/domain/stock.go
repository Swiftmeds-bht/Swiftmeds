package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidStockKey             = errors.New("pharmacy ID and product ID are required")
	ErrNegativeStock               = errors.New("stock quantities cannot be negative")
	ErrInvalidReservationQuantity  = errors.New("reservation quantity must be positive")
	ErrInsufficientAvailableStock  = errors.New("insufficient available stock")
	ErrInsufficientReservedStock   = errors.New("insufficient reserved stock")
	ErrOnHandBelowReservedQuantity = errors.New("on-hand quantity cannot be below reserved quantity")
)

type Stock struct {
	PharmacyID       string
	ProductID        string
	QuantityOnHand   int32
	QuantityReserved int32
	ConfirmedAt      time.Time
}

func NewStock(
	pharmacyID string,
	productID string,
	onHand int32,
	reserved int32,
	confirmedAt time.Time,
) (Stock, error) {
	if strings.TrimSpace(pharmacyID) == "" || strings.TrimSpace(productID) == "" {
		return Stock{}, ErrInvalidStockKey
	}
	if onHand < 0 || reserved < 0 {
		return Stock{}, ErrNegativeStock
	}
	if reserved > onHand {
		return Stock{}, ErrOnHandBelowReservedQuantity
	}

	return Stock{
		PharmacyID:       pharmacyID,
		ProductID:        productID,
		QuantityOnHand:   onHand,
		QuantityReserved: reserved,
		ConfirmedAt:      confirmedAt,
	}, nil
}

func (s Stock) QuantityAvailable() int32 {
	return s.QuantityOnHand - s.QuantityReserved
}

func (s Stock) SetQuantityOnHand(quantity int32) (Stock, error) {
	if quantity < 0 {
		return s, ErrNegativeStock
	}
	if quantity < s.QuantityReserved {
		return s, ErrOnHandBelowReservedQuantity
	}

	s.QuantityOnHand = quantity
	return s, nil
}

func (s Stock) Reserve(quantity int32) (Stock, error) {
	if quantity <= 0 {
		return s, ErrInvalidReservationQuantity
	}
	if quantity > s.QuantityAvailable() {
		return s, ErrInsufficientAvailableStock
	}

	s.QuantityReserved += quantity
	return s, nil
}

func (s Stock) CommitReservation(quantity int32) (Stock, error) {
	if quantity <= 0 {
		return s, ErrInvalidReservationQuantity
	}
	if quantity > s.QuantityReserved {
		return s, ErrInsufficientReservedStock
	}

	s.QuantityOnHand -= quantity
	s.QuantityReserved -= quantity
	return s, nil
}

func (s Stock) ReleaseReservation(quantity int32) (Stock, error) {
	if quantity <= 0 {
		return s, ErrInvalidReservationQuantity
	}
	if quantity > s.QuantityReserved {
		return s, ErrInsufficientReservedStock
	}

	s.QuantityReserved -= quantity
	return s, nil
}
