// Package domain holds Trust's rules. Pure computation: no I/O, no clock,
// standard library only.
//
// A pharmacy's trust score says how often patients who collected a medicine
// found it really was in stock, with recent feedback counting more:
//
//	score = (prior·0.5 + Σ wᵢ·xᵢ) / (prior + Σ wᵢ)
//
// where:
//   - xᵢ is 1 when feedback i said "was in stock" and 0 when it said "not in stock";
//   - wᵢ = 0.5^(ageᵢ / half-life), so feedback half a half-life old counts about
//     0.7, one half-life old 0.5, two half-lives old 0.25;
//   - prior is a number of imaginary neutral (0.5) answers every pharmacy starts with.
//
// So a new pharmacy is exactly neutral (0.5), one answer cannot swing it to
// 0 or 1, and when feedback stops coming the score drifts back to neutral. The
// score is always in 0..1 (MinScore..MaxScore), which is what Ranking expects.
package domain

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	// NeutralScore is the score of a pharmacy without feedback. Search uses the
	// same value when Trust does not answer.
	NeutralScore = 0.5
	// MinScore and MaxScore bound every score.
	MinScore = 0.0
	MaxScore = 1.0

	// MaxBatch bounds one BatchGetTrust request (Search sends at most 100 pharmacies).
	MaxBatch = 1000
)

var (
	ErrMissingPharmacyID = errors.New("pharmacy_id is required")
	ErrMissingFeedbackID = errors.New("feedback_id is required")
	ErrMissingOccurredAt = errors.New("feedback time is required")
	ErrTooManyPharmacies = fmt.Errorf("at most %d pharmacy_ids per request", MaxBatch)
	ErrInvalidParams     = errors.New("trust half-life must be positive and the prior weight finite and not negative")
)

// ValidatePharmacyIDs checks one batch request: at most MaxBatch IDs, none blank.
func ValidatePharmacyIDs(ids []string) error {
	if len(ids) > MaxBatch {
		return ErrTooManyPharmacies
	}
	for i, id := range ids {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("pharmacy_ids[%d]: %w", i, ErrMissingPharmacyID)
		}
	}
	return nil
}

// Params tune the score.
type Params struct {
	// HalfLife is how long it takes a piece of feedback to count half as much.
	HalfLife time.Duration
	// PriorWeight is how many neutral answers a pharmacy starts with. 0 means
	// the score is the plain weighted share of "was in stock" answers.
	PriorWeight float64
}

// DefaultParams: feedback counts half after 30 days; a new pharmacy starts as
// if it had 3 neutral answers (one "not in stock" then gives 0.375).
var DefaultParams = Params{HalfLife: 30 * 24 * time.Hour, PriorWeight: 3}

func (p Params) Validate() error {
	if p.HalfLife <= 0 || math.IsNaN(p.PriorWeight) || math.IsInf(p.PriorWeight, 0) || p.PriorWeight < 0 {
		return ErrInvalidParams
	}
	return nil
}

// Feedback is one feedback.submitted event, reduced to what Trust uses. The
// rating is about the pickup experience, not about stock accuracy, so it is
// not part of the score.
type Feedback struct {
	ID         string
	PharmacyID string
	WasInStock bool
	OccurredAt time.Time
}

func (f Feedback) Validate() error {
	if strings.TrimSpace(f.ID) == "" {
		return ErrMissingFeedbackID
	}
	if strings.TrimSpace(f.PharmacyID) == "" {
		return ErrMissingPharmacyID
	}
	if f.OccurredAt.IsZero() {
		return ErrMissingOccurredAt
	}
	return nil
}

// Record is what Trust stores for one pharmacy. The zero Record (plus a
// PharmacyID) is a pharmacy without feedback.
type Record struct {
	PharmacyID    string
	FeedbackCount int32
	InStockCount  int32
	// WeightedInStock and WeightedTotal are Σ wᵢ·xᵢ and Σ wᵢ with every weight
	// taken at LastAt. Keeping the sums instead of every answer makes an update
	// O(1), and moving the reference time only multiplies both by one factor.
	WeightedInStock float64
	WeightedTotal   float64
	// LastAt is when the newest feedback was given; zero without feedback.
	LastAt time.Time
}

// Apply adds one piece of feedback. Feedback may arrive out of order: an
// older answer is weighted by its age at LastAt, so the result does not depend
// on the order in which the answers were applied.
func (r Record) Apply(f Feedback, p Params) Record {
	x := 0.0
	if f.WasInStock {
		x = 1
	}
	switch {
	case r.LastAt.IsZero():
		r.WeightedInStock, r.WeightedTotal, r.LastAt = x, 1, f.OccurredAt
	case f.OccurredAt.After(r.LastAt):
		k := decay(f.OccurredAt.Sub(r.LastAt), p.HalfLife)
		r.WeightedInStock = r.WeightedInStock*k + x
		r.WeightedTotal = r.WeightedTotal*k + 1
		r.LastAt = f.OccurredAt
	default:
		w := decay(r.LastAt.Sub(f.OccurredAt), p.HalfLife)
		r.WeightedInStock += w * x
		r.WeightedTotal += w
	}
	r.FeedbackCount = increment(r.FeedbackCount)
	if f.WasInStock {
		r.InStockCount = increment(r.InStockCount)
	}
	return r
}

// Score is the record's trust score at now, always in MinScore..MaxScore.
func (r Record) Score(now time.Time, p Params) float64 {
	d := 0.0
	if !r.LastAt.IsZero() {
		d = decay(now.Sub(r.LastAt), p.HalfLife)
	}
	num := p.PriorWeight*NeutralScore + d*r.WeightedInStock
	den := p.PriorWeight + d*r.WeightedTotal
	if den <= 0 || math.IsNaN(num/den) {
		return NeutralScore
	}
	return math.Min(MaxScore, math.Max(MinScore, num/den))
}

// Trust is a pharmacy's score as the API returns it.
type Trust struct {
	PharmacyID    string
	Score         float64
	FeedbackCount int32
	InStockCount  int32
	// UpdatedAt is when the newest feedback was given; zero without feedback.
	UpdatedAt time.Time
}

// Trust is the record's score at now.
func (r Record) Trust(now time.Time, p Params) Trust {
	return Trust{
		PharmacyID:    r.PharmacyID,
		Score:         r.Score(now, p),
		FeedbackCount: r.FeedbackCount,
		InStockCount:  r.InStockCount,
		UpdatedAt:     r.LastAt,
	}
}

// decay is 0.5^(age/halfLife). Feedback "from the future" (clock skew) counts fully.
func decay(age, halfLife time.Duration) float64 {
	if age <= 0 {
		return 1
	}
	return math.Exp2(-float64(age) / float64(halfLife))
}

// increment adds one, stopping at the int32 maximum instead of wrapping around.
func increment(n int32) int32 {
	if n == math.MaxInt32 {
		return n
	}
	return n + 1
}
