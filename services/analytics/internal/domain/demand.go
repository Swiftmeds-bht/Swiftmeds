package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidRange   = errors.New("invalid time range")
	ErrMissingProduct = errors.New("product_id is required")
)

type SearchRecorded struct {
	EventID, SearchID, ProductID, ProductName, Dzongkhag string
	OccurredAt                                           time.Time
	HasStock                                             bool
}

type StockUnavailable struct {
	EventID, PharmacyID, ProductID, ProductName, Dzongkhag string
	OccurredAt                                             time.Time
}

type DemandQuery struct {
	From, To  time.Time
	Dzongkhag string
	Limit     int
}

type DemandItem struct {
	ProductID, ProductName  string
	SearchCount, UnmetCount int32
}

func (q DemandQuery) Validate() error {
	if q.From.IsZero() || q.To.IsZero() || !q.From.Before(q.To) {
		return ErrInvalidRange
	}
	if q.Limit <= 0 {
		return errors.New("limit must be positive")
	}
	return nil
}
