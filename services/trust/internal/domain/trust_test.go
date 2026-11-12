package domain

import (
	"errors"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

var (
	t0  = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	day = 24 * time.Hour
)

func fb(id string, inStock bool, at time.Time) Feedback {
	return Feedback{ID: id, PharmacyID: "ph-1", WasInStock: inStock, OccurredAt: at}
}

// apply adds n answers, all inStock or all not, given at the same time.
func apply(r Record, p Params, n int, inStock bool, at time.Time) Record {
	for i := range n {
		r = r.Apply(fb("fb-"+string(rune('a'+i%26)), inStock, at), p)
	}
	return r
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

func TestNewPharmacyStartsNeutral(t *testing.T) {
	for name, p := range map[string]Params{
		"default params": DefaultParams,
		"no prior":       {HalfLife: day, PriorWeight: 0},
		"big prior":      {HalfLife: day, PriorWeight: 1e6},
	} {
		t.Run(name, func(t *testing.T) {
			r := Record{PharmacyID: "ph-new"}
			got := r.Trust(t0, p)
			want := Trust{PharmacyID: "ph-new", Score: NeutralScore}
			if got != want {
				t.Errorf("Trust = %+v, want %+v", got, want)
			}
		})
	}
	if NeutralScore != 0.5 {
		t.Errorf("NeutralScore = %v, want 0.5 (Search falls back to the same value)", NeutralScore)
	}
}

// Boundary tests: the score reaches exactly MinScore (0) and MaxScore (1) when
// nothing pulls it towards neutral, and never goes past either.
func TestScoreBoundaries(t *testing.T) {
	noPrior := Params{HalfLife: 30 * day, PriorWeight: 0}
	tests := []struct {
		name    string
		p       Params
		n       int
		inStock bool
		check   func(float64) bool
		want    string
	}{
		{"no prior, one 'not in stock' is exactly 0", noPrior, 1, false, func(s float64) bool { return s == MinScore }, "== 0"},
		{"no prior, 1000 'not in stock' is exactly 0", noPrior, 1000, false, func(s float64) bool { return s == MinScore }, "== 0"},
		{"no prior, one 'in stock' is exactly the max", noPrior, 1, true, func(s float64) bool { return s == MaxScore }, "== 1"},
		{"no prior, 1000 'in stock' is exactly the max", noPrior, 1000, true, func(s float64) bool { return s == MaxScore }, "== 1"},
		{"default prior, 1000 'not in stock' is just above 0", DefaultParams, 1000, false,
			func(s float64) bool { return s > MinScore && s < 0.002 }, "in (0, 0.002)"},
		{"default prior, 1000 'in stock' is just below the max", DefaultParams, 1000, true,
			func(s float64) bool { return s < MaxScore && s > 0.998 }, "in (0.998, 1)"},
		{"default prior, one 'not in stock' is 0.375", DefaultParams, 1, false,
			func(s float64) bool { return near(s, 1.5/4) }, "0.375"},
		{"default prior, one 'in stock' is 0.625", DefaultParams, 1, true,
			func(s float64) bool { return near(s, 2.5/4) }, "0.625"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := apply(Record{PharmacyID: "ph-1"}, tt.p, tt.n, tt.inStock, t0)
			if s := r.Score(t0, tt.p); !tt.check(s) {
				t.Errorf("score = %v, want %s", s, tt.want)
			}
		})
	}
}

// Whatever the answers, times and parameters, the score stays in 0..1 and is a number.
func TestScoreNeverLeavesTheUnitInterval(t *testing.T) {
	rng := rand.New(rand.NewPCG(73, 73))
	params := []Params{
		DefaultParams,
		{HalfLife: 1, PriorWeight: 0},
		{HalfLife: time.Duration(math.MaxInt64), PriorWeight: 0},
		{HalfLife: day, PriorWeight: 1e300},
		{HalfLife: time.Hour, PriorWeight: 0.001},
	}
	for _, p := range params {
		for range 200 {
			r := Record{PharmacyID: "ph-1"}
			for i := range 1 + rng.IntN(50) {
				at := t0.Add(time.Duration(rng.Int64N(int64(3650*day))) - 1825*day)
				r = r.Apply(fb("fb-"+string(rune(i)), rng.IntN(2) == 0, at), p)
				now := t0.Add(time.Duration(rng.Int64N(int64(7300 * day))))
				if s := r.Score(now, p); math.IsNaN(s) || s < MinScore || s > MaxScore {
					t.Fatalf("params %+v record %+v: score %v outside 0..1", p, r, s)
				}
			}
		}
	}
}

func TestRecentFeedbackCountsMore(t *testing.T) {
	p := DefaultParams
	now := t0.Add(60 * day)
	oldBadRecentGood := Record{}.Apply(fb("old", false, t0), p).Apply(fb("new", true, now), p)
	oldGoodRecentBad := Record{}.Apply(fb("old", true, t0), p).Apply(fb("new", false, now), p)

	good, bad := oldBadRecentGood.Score(now, p), oldGoodRecentBad.Score(now, p)
	if good <= NeutralScore || bad >= NeutralScore {
		t.Fatalf("recent 'in stock' = %v (want > 0.5), recent 'not in stock' = %v (want < 0.5)", good, bad)
	}
	// Mirror images: same distance from neutral.
	if !near(good-NeutralScore, NeutralScore-bad) {
		t.Errorf("scores %v and %v are not symmetric around 0.5", good, bad)
	}
}

// With no prior: "in stock" at t0, "not in stock" one half-life later. At that
// moment the first weighs 0.5 and the second 1, so the score is 0.5/1.5.
func TestHalfLife(t *testing.T) {
	p := Params{HalfLife: 30 * day, PriorWeight: 0}
	r := Record{}.Apply(fb("a", true, t0), p).Apply(fb("b", false, t0.Add(30*day)), p)
	if s := r.Score(t0.Add(30*day), p); !near(s, 1.0/3) {
		t.Errorf("score = %v, want 1/3", s)
	}
	// With no prior, waiting changes nothing: both weights shrink by the same factor.
	if s := r.Score(t0.Add(90*day), p); !near(s, 1.0/3) {
		t.Errorf("score later = %v, want 1/3", s)
	}
}

func TestOldFeedbackFadesBackToNeutral(t *testing.T) {
	p := DefaultParams
	r := apply(Record{}, p, 5, false, t0)
	prev := r.Score(t0, p)
	for _, after := range []time.Duration{p.HalfLife, 3 * p.HalfLife, 10 * p.HalfLife} {
		s := r.Score(t0.Add(after), p)
		if s <= prev || s >= NeutralScore {
			t.Errorf("after %v: score %v, want between %v and 0.5", after, s, prev)
		}
		prev = s
	}
	// After 2000 half-lives the weight underflows to 0: exactly neutral again.
	if s := r.Score(t0.Add(2000*p.HalfLife), p); s != NeutralScore {
		t.Errorf("long after: score %v, want exactly 0.5", s)
	}
}

func TestOrderOfFeedbackDoesNotMatter(t *testing.T) {
	p := DefaultParams
	answers := []Feedback{
		fb("a", true, t0), fb("b", false, t0.Add(3*day)), fb("c", false, t0.Add(10*day)),
		fb("d", true, t0.Add(45*day)), fb("e", true, t0.Add(45*day)), fb("f", false, t0.Add(-20*day)),
	}
	now := t0.Add(50 * day)
	var want float64
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range 50 {
		rng.Shuffle(len(answers), func(a, b int) { answers[a], answers[b] = answers[b], answers[a] })
		r := Record{}
		for _, f := range answers {
			r = r.Apply(f, p)
		}
		s := r.Score(now, p)
		if i == 0 {
			want = s
			if !r.LastAt.Equal(t0.Add(45*day)) || r.FeedbackCount != 6 || r.InStockCount != 3 {
				t.Fatalf("record = %+v", r)
			}
		} else if !near(s, want) {
			t.Fatalf("order %d: score %v, first order gave %v", i, s, want)
		}
	}
}

func TestFeedbackFromTheFutureCountsFully(t *testing.T) {
	p := Params{HalfLife: day, PriorWeight: 0}
	r := Record{}.Apply(fb("a", false, t0.Add(time.Hour)), p).Apply(fb("b", true, t0.Add(time.Hour)), p)
	// Asked before the feedback time (clock skew): no decay, no NaN.
	if s := r.Score(t0, p); !near(s, 0.5) {
		t.Errorf("score = %v, want 0.5", s)
	}
}

func TestCountsStopAtTheInt32Maximum(t *testing.T) {
	r := Record{FeedbackCount: math.MaxInt32 - 1, InStockCount: math.MaxInt32 - 1, WeightedInStock: 1, WeightedTotal: 1, LastAt: t0}
	r = r.Apply(fb("a", true, t0), DefaultParams)
	if r.FeedbackCount != math.MaxInt32 || r.InStockCount != math.MaxInt32 {
		t.Fatalf("counts = %d, %d; want the maximum", r.FeedbackCount, r.InStockCount)
	}
	r = r.Apply(fb("b", true, t0), DefaultParams)
	if r.FeedbackCount != math.MaxInt32 || r.InStockCount != math.MaxInt32 {
		t.Errorf("counts wrapped around: %d, %d", r.FeedbackCount, r.InStockCount)
	}
	r = r.Apply(fb("c", false, t0), DefaultParams)
	if r.InStockCount != math.MaxInt32 {
		t.Errorf("'not in stock' changed the in-stock count: %d", r.InStockCount)
	}
}

func TestParamsValidate(t *testing.T) {
	tests := []struct {
		name string
		p    Params
		ok   bool
	}{
		{"default", DefaultParams, true},
		{"no prior", Params{HalfLife: 1, PriorWeight: 0}, true},
		{"zero half-life", Params{HalfLife: 0, PriorWeight: 1}, false},
		{"negative half-life", Params{HalfLife: -time.Hour, PriorWeight: 1}, false},
		{"negative prior", Params{HalfLife: day, PriorWeight: -0.001}, false},
		{"NaN prior", Params{HalfLife: day, PriorWeight: math.NaN()}, false},
		{"infinite prior", Params{HalfLife: day, PriorWeight: math.Inf(1)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate()
			if tt.ok != (err == nil) || (err != nil && !errors.Is(err, ErrInvalidParams)) {
				t.Errorf("Validate(%+v) = %v, want ok=%v", tt.p, err, tt.ok)
			}
		})
	}
}

func TestFeedbackValidate(t *testing.T) {
	ok := fb("fb-1", true, t0)
	tests := map[string]struct {
		f    Feedback
		want error
	}{
		"valid":            {ok, nil},
		"no feedback id":   {Feedback{PharmacyID: "ph-1", OccurredAt: t0}, ErrMissingFeedbackID},
		"blank pharmacy":   {Feedback{ID: "fb-1", PharmacyID: "  ", OccurredAt: t0}, ErrMissingPharmacyID},
		"no feedback time": {Feedback{ID: "fb-1", PharmacyID: "ph-1"}, ErrMissingOccurredAt},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if err := tt.f.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidatePharmacyIDs(t *testing.T) {
	ids := func(n int) []string { return strings.Split(strings.Repeat("ph,", n), ",")[:n] }
	if err := ValidatePharmacyIDs(nil); err != nil {
		t.Errorf("no IDs: %v", err)
	}
	if err := ValidatePharmacyIDs(ids(MaxBatch)); err != nil {
		t.Errorf("%d IDs: %v", MaxBatch, err)
	}
	if err := ValidatePharmacyIDs(ids(MaxBatch + 1)); !errors.Is(err, ErrTooManyPharmacies) {
		t.Errorf("%d IDs: %v, want ErrTooManyPharmacies", MaxBatch+1, err)
	}
	if err := ValidatePharmacyIDs([]string{"ph-1", " "}); !errors.Is(err, ErrMissingPharmacyID) || !strings.Contains(err.Error(), "[1]") {
		t.Errorf("blank ID: %v, want ErrMissingPharmacyID naming index 1", err)
	}
}
