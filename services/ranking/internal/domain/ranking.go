// Package domain holds Ranking's rules. Pure computation: no I/O, no clock,
// standard library only. Only objective factors count; there is no paid placement.
//
//	score = w.Distance*distance + w.Freshness*freshness + w.Trust*trust + w.Price*price
//
// Every component is in 0..1 where 1 is best:
//   - distance and price are min-max normalised across the candidate set
//     (nearest / cheapest = 1, farthest / dearest = 0; all equal = 1);
//   - freshness and trust are already 0..1 scores and are used as given.
package domain

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
)

// MaxCandidates bounds one request (Search sends at most a few hundred).
const MaxCandidates = 1000

// WeightSumTolerance allows for floating-point error in weights such as
// 0.1+0.2+... It is 2^-20 (about 9.5e-7), written as a hex float: an exact
// binary fraction, so tests can put a sum exactly on the boundary.
const WeightSumTolerance = 0x1p-20

var (
	ErrInvalidWeights    = errors.New("weights must be finite, non-negative and sum to 1")
	ErrInvalidCandidate  = errors.New("invalid candidate")
	ErrTooManyCandidates = fmt.Errorf("at most %d candidates per request", MaxCandidates)
)

// Weights say how much each factor counts. They must sum to 1.
type Weights struct {
	Distance, Freshness, Trust, Price float64
}

// DefaultWeights favour nearby pharmacies with recently confirmed stock.
var DefaultWeights = Weights{Distance: 0.40, Freshness: 0.25, Trust: 0.20, Price: 0.15}

func (w Weights) Validate() error {
	sum := 0.0
	for _, v := range []float64{w.Distance, w.Freshness, w.Trust, w.Price} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return ErrInvalidWeights
		}
		sum += v
	}
	if math.Abs(sum-1) > WeightSumTolerance {
		return ErrInvalidWeights
	}
	return nil
}

// Candidate is one pharmacy offering one product.
type Candidate struct {
	PharmacyID string
	ProductID  string
	DistanceM  int32
	// Freshness and Trust are 0..1.
	Freshness float64
	Trust     float64
	// PriceChhertum 0 means the price is unknown.
	PriceChhertum int64
	// Quantity 0 means out of stock: the candidate is dropped.
	Quantity int32
}

// validate uses plain if statements rather than a tagless switch: Go's coverage
// profile does not cover switch case expressions, so mutation testing could
// not tell that these checks are tested.
func (c Candidate) validate() error {
	if c.PharmacyID == "" {
		return fmt.Errorf("%w: pharmacy_id is required", ErrInvalidCandidate)
	}
	if c.ProductID == "" {
		return fmt.Errorf("%w: product_id is required", ErrInvalidCandidate)
	}
	if c.DistanceM < 0 {
		return fmt.Errorf("%w: distance_m must not be negative", ErrInvalidCandidate)
	}
	if !unitInterval(c.Freshness) {
		return fmt.Errorf("%w: freshness_score must be between 0 and 1", ErrInvalidCandidate)
	}
	if !unitInterval(c.Trust) {
		return fmt.Errorf("%w: trust_score must be between 0 and 1", ErrInvalidCandidate)
	}
	if c.PriceChhertum < 0 {
		return fmt.Errorf("%w: price_chhertum must not be negative", ErrInvalidCandidate)
	}
	if c.Quantity < 0 {
		return fmt.Errorf("%w: quantity_available must not be negative", ErrInvalidCandidate)
	}
	return nil
}

func unitInterval(v float64) bool { return v >= 0 && v <= 1 }

// Breakdown is each factor's weighted contribution; the four add up to the score.
type Breakdown struct {
	Distance, Freshness, Trust, Price float64
}

// Ranked is a candidate with its score.
type Ranked struct {
	Candidate
	Score     float64
	Breakdown Breakdown
}

// Rank validates the input, drops out-of-stock candidates and returns the rest
// highest score first. Ties are broken by distance, then pharmacy ID, then
// product ID, so the order never depends on the input order.
func Rank(candidates []Candidate, w Weights) ([]Ranked, error) {
	if err := w.Validate(); err != nil {
		return nil, err
	}
	if len(candidates) > MaxCandidates {
		return nil, ErrTooManyCandidates
	}
	inStock := make([]Candidate, 0, len(candidates))
	for i, c := range candidates {
		if err := c.validate(); err != nil {
			return nil, fmt.Errorf("candidate %d: %w", i, err)
		}
		if c.Quantity > 0 {
			inStock = append(inStock, c)
		}
	}
	if len(inStock) == 0 {
		return []Ranked{}, nil
	}

	distances := make([]float64, len(inStock))
	for i, c := range inStock {
		distances[i] = float64(c.DistanceM)
	}
	distanceScores := lowerIsBetter(distances)
	priceScores := lowerIsBetter(effectivePrices(inStock))

	out := make([]Ranked, len(inStock))
	for i, c := range inStock {
		b := Breakdown{
			Distance:  w.Distance * distanceScores[i],
			Freshness: w.Freshness * c.Freshness,
			Trust:     w.Trust * c.Trust,
			Price:     w.Price * priceScores[i],
		}
		out[i] = Ranked{Candidate: c, Score: b.Distance + b.Freshness + b.Trust + b.Price, Breakdown: b}
	}

	slices.SortFunc(out, func(a, b Ranked) int {
		return cmp.Or(
			cmp.Compare(b.Score, a.Score), // highest first
			cmp.Compare(a.DistanceM, b.DistanceM),
			cmp.Compare(a.PharmacyID, b.PharmacyID),
			cmp.Compare(a.ProductID, b.ProductID),
		)
	})
	return out, nil
}

// lowerIsBetter min-max normalises values so the smallest gets 1 and the
// largest 0. If they are all equal, every value gets 1.
func lowerIsBetter(values []float64) []float64 {
	lo, hi := slices.Min(values), slices.Max(values)
	out := make([]float64, len(values))
	for i, v := range values {
		if hi == lo {
			out[i] = 1
		} else {
			out[i] = (hi - v) / (hi - lo)
		}
	}
	return out
}

// effectivePrices replaces unknown prices (0) with the median of the known
// ones, so a pharmacy is neither rewarded nor punished for not listing a price.
// With no known prices at all, every candidate gets the same price.
func effectivePrices(cs []Candidate) []float64 {
	var known []float64
	for _, c := range cs {
		if c.PriceChhertum > 0 {
			known = append(known, float64(c.PriceChhertum))
		}
	}
	m := median(known)
	out := make([]float64, len(cs))
	for i, c := range cs {
		if c.PriceChhertum > 0 {
			out[i] = float64(c.PriceChhertum)
		} else {
			out[i] = m
		}
	}
	return out
}

// median of values (mean of the middle two for an even count); 0 when empty.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := slices.Clone(values)
	slices.Sort(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}
