package domain

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

const eps = 1e-9

func near(a, b float64) bool { return math.Abs(a-b) < eps }

// cand builds an in-stock candidate with neutral freshness/trust and a price.
func cand(id string, distance int32, price int64) Candidate {
	return Candidate{PharmacyID: id, ProductID: "pr-1", DistanceM: distance, Freshness: 0.5, Trust: 0.5, PriceChhertum: price, Quantity: 1}
}

func ids(rs []Ranked) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.PharmacyID
	}
	return out
}

func mustRank(t *testing.T, cs []Candidate, w Weights) []Ranked {
	t.Helper()
	got, err := Rank(cs, w)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// --- Weights ---------------------------------------------------------------

func TestDefaultWeightsAreValid(t *testing.T) {
	if err := DefaultWeights.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestWeightsValidate(t *testing.T) {
	cases := []struct {
		name string
		w    Weights
		ok   bool
	}{
		{"sums to 1", Weights{0.25, 0.25, 0.25, 0.25}, true},
		{"one factor only", Weights{1, 0, 0, 0}, true},
		{"float rounding within tolerance", Weights{0.1, 0.2, 0.3, 0.4 + 1e-9}, true},
		{"sums to slightly more than 1", Weights{0.25, 0.25, 0.25, 0.26}, false},
		{"sums to slightly less than 1", Weights{0.25, 0.25, 0.25, 0.24}, false},
		{"all zero", Weights{}, false},
		{"negative weight", Weights{1.5, -0.5, 0, 0}, false},
		{"NaN", Weights{math.NaN(), 0.5, 0.25, 0.25}, false},
		{"infinite", Weights{math.Inf(1), 0, 0, 0}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.w.Validate()
			if tc.ok && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidWeights) {
				t.Errorf("Validate() = %v, want ErrInvalidWeights", err)
			}
		})
	}
}

// --- Input validation (equivalence partitions + boundaries) -----------------

func TestRankRejectsInvalidCandidates(t *testing.T) {
	ok := cand("ph-1", 100, 500)
	mod := func(f func(*Candidate)) Candidate { c := ok; f(&c); return c }
	cases := map[string]Candidate{
		"missing pharmacy":  mod(func(c *Candidate) { c.PharmacyID = "" }),
		"missing product":   mod(func(c *Candidate) { c.ProductID = "" }),
		"negative distance": mod(func(c *Candidate) { c.DistanceM = -1 }),
		"freshness below 0": mod(func(c *Candidate) { c.Freshness = -0.0001 }),
		"freshness above 1": mod(func(c *Candidate) { c.Freshness = 1.0001 }),
		"freshness NaN":     mod(func(c *Candidate) { c.Freshness = math.NaN() }),
		"trust below 0":     mod(func(c *Candidate) { c.Trust = -0.0001 }),
		"trust above 1":     mod(func(c *Candidate) { c.Trust = 1.0001 }),
		"negative price":    mod(func(c *Candidate) { c.PriceChhertum = -1 }),
		"negative quantity": mod(func(c *Candidate) { c.Quantity = -1 }),
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Rank([]Candidate{ok, c}, DefaultWeights)
			if !errors.Is(err, ErrInvalidCandidate) {
				t.Fatalf("err = %v, want ErrInvalidCandidate", err)
			}
			if !strings.Contains(err.Error(), "candidate 1") {
				t.Errorf("error %q does not say which candidate", err)
			}
		})
	}
}

func TestRankAcceptsBoundaryValues(t *testing.T) {
	edge := []Candidate{
		{PharmacyID: "a", ProductID: "p", DistanceM: 0, Freshness: 0, Trust: 0, PriceChhertum: 0, Quantity: 1},
		{PharmacyID: "b", ProductID: "p", DistanceM: math.MaxInt32, Freshness: 1, Trust: 1, PriceChhertum: math.MaxInt32, Quantity: math.MaxInt32},
	}
	if _, err := Rank(edge, DefaultWeights); err != nil {
		t.Fatalf("boundary values rejected: %v", err)
	}
}

func TestRankRejectsInvalidWeights(t *testing.T) {
	if _, err := Rank([]Candidate{cand("a", 1, 1)}, Weights{}); !errors.Is(err, ErrInvalidWeights) {
		t.Fatalf("err = %v, want ErrInvalidWeights", err)
	}
}

func TestRankCandidateLimit(t *testing.T) {
	cs := make([]Candidate, MaxCandidates+1)
	for i := range cs {
		cs[i] = cand("ph", 1, 1)
	}
	if _, err := Rank(cs, DefaultWeights); !errors.Is(err, ErrTooManyCandidates) {
		t.Fatalf("%d candidates: err = %v, want ErrTooManyCandidates", len(cs), err)
	}
	if _, err := Rank(cs[:MaxCandidates], DefaultWeights); err != nil {
		t.Fatalf("%d candidates: %v", MaxCandidates, err)
	}
}

// --- Out of stock -----------------------------------------------------------

func TestOutOfStockIsDroppedEvenIfOtherwiseBest(t *testing.T) {
	best := Candidate{PharmacyID: "best-but-empty", ProductID: "p", DistanceM: 0, Freshness: 1, Trust: 1, PriceChhertum: 1, Quantity: 0}
	worse := Candidate{PharmacyID: "in-stock", ProductID: "p", DistanceM: 9000, Freshness: 0, Trust: 0, PriceChhertum: 99999, Quantity: 1}

	got := mustRank(t, []Candidate{best, worse}, DefaultWeights)
	if want := []string{"in-stock"}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("ranked %v, want %v", ids(got), want)
	}
}

func TestEmptyAndAllOutOfStock(t *testing.T) {
	if got := mustRank(t, nil, DefaultWeights); len(got) != 0 {
		t.Errorf("nil input: %v", got)
	}
	out := cand("a", 1, 1)
	out.Quantity = 0
	if got := mustRank(t, []Candidate{out}, DefaultWeights); len(got) != 0 {
		t.Errorf("all out of stock: %v", got)
	}
}

// --- Normalisation -------------------------------------------------------------

func TestDistanceIsMinMaxNormalised(t *testing.T) {
	w := Weights{Distance: 1}
	got := mustRank(t, []Candidate{cand("far", 3000, 0), cand("mid", 2000, 0), cand("near", 1000, 0)}, w)
	want := map[string]float64{"near": 1, "mid": 0.5, "far": 0}
	for _, r := range got {
		if !near(r.Score, want[r.PharmacyID]) {
			t.Errorf("%s score = %v, want %v", r.PharmacyID, r.Score, want[r.PharmacyID])
		}
	}
	if !reflect.DeepEqual(ids(got), []string{"near", "mid", "far"}) {
		t.Errorf("order = %v", ids(got))
	}
}

func TestEqualValuesAllGetFullMarks(t *testing.T) {
	w := Weights{Distance: 0.5, Price: 0.5}
	for _, r := range mustRank(t, []Candidate{cand("a", 700, 400), cand("b", 700, 400)}, w) {
		if !near(r.Score, 1) {
			t.Errorf("%s score = %v, want 1 when all distances and prices are equal", r.PharmacyID, r.Score)
		}
	}
	if r := mustRank(t, []Candidate{cand("only", 5000, 900)}, w); !near(r[0].Score, 1) {
		t.Errorf("single candidate score = %v, want 1", r[0].Score)
	}
}

func TestPriceIsMinMaxNormalisedCheapestBest(t *testing.T) {
	w := Weights{Price: 1}
	got := mustRank(t, []Candidate{cand("dear", 1, 900), cand("cheap", 1, 300), cand("mid", 1, 600)}, w)
	want := map[string]float64{"cheap": 1, "mid": 0.5, "dear": 0}
	for _, r := range got {
		if !near(r.Score, want[r.PharmacyID]) {
			t.Errorf("%s score = %v, want %v", r.PharmacyID, r.Score, want[r.PharmacyID])
		}
	}
}

func TestUnknownPriceScoresAsTheMedian(t *testing.T) {
	w := Weights{Price: 1}
	// Known prices 100, 300, 500 -> median 300 -> the unknown one scores like 300 (0.5).
	got := mustRank(t, []Candidate{cand("a", 1, 100), cand("b", 1, 300), cand("c", 1, 500), cand("unknown", 1, 0)}, w)
	for _, r := range got {
		if r.PharmacyID == "unknown" && !near(r.Score, 0.5) {
			t.Errorf("unknown price score = %v, want 0.5 (median)", r.Score)
		}
	}
	// Even count: known 100, 200, 400, 500 -> median 300 -> (500-300)/(500-100) = 0.5.
	got = mustRank(t, []Candidate{cand("a", 1, 100), cand("b", 1, 200), cand("c", 1, 400), cand("d", 1, 500), cand("unknown", 1, 0)}, w)
	for _, r := range got {
		if r.PharmacyID == "unknown" && !near(r.Score, 0.5) {
			t.Errorf("even-count median: unknown price score = %v, want 0.5", r.Score)
		}
	}
	// No known prices at all: everyone is equal on price.
	for _, r := range mustRank(t, []Candidate{cand("x", 1, 0), cand("y", 1, 0)}, w) {
		if !near(r.Score, 1) {
			t.Errorf("no known prices: %s score = %v, want 1", r.PharmacyID, r.Score)
		}
	}
}

func TestFreshnessAndTrustAreUsedAsGiven(t *testing.T) {
	a := cand("a", 1, 1)
	a.Freshness, a.Trust = 0.8, 0.3
	b := cand("b", 1, 1)
	b.Freshness, b.Trust = 0.6, 0.9

	got := mustRank(t, []Candidate{a, b}, Weights{Freshness: 0.5, Trust: 0.5})
	want := map[string]float64{"a": 0.55, "b": 0.75}
	for _, r := range got {
		if !near(r.Score, want[r.PharmacyID]) {
			t.Errorf("%s score = %v, want %v", r.PharmacyID, r.Score, want[r.PharmacyID])
		}
	}
}

// --- Score and breakdown --------------------------------------------------------

func TestWorkedExampleWithDefaultWeights(t *testing.T) {
	near1 := Candidate{PharmacyID: "near", ProductID: "p", DistanceM: 500, Freshness: 0.5, Trust: 0.5, PriceChhertum: 600, Quantity: 3}
	far := Candidate{PharmacyID: "far", ProductID: "p", DistanceM: 1500, Freshness: 1.0, Trust: 1.0, PriceChhertum: 400, Quantity: 3}

	got := mustRank(t, []Candidate{near1, far}, DefaultWeights)
	// near: 0.40*1 + 0.25*0.5 + 0.20*0.5 + 0.15*0 = 0.625
	// far:  0.40*0 + 0.25*1   + 0.20*1   + 0.15*1 = 0.600
	if !reflect.DeepEqual(ids(got), []string{"near", "far"}) {
		t.Fatalf("order = %v", ids(got))
	}
	if !near(got[0].Score, 0.625) || !near(got[1].Score, 0.600) {
		t.Errorf("scores = %v, %v; want 0.625, 0.600", got[0].Score, got[1].Score)
	}
	b := got[0].Breakdown
	if !near(b.Distance, 0.40) || !near(b.Freshness, 0.125) || !near(b.Trust, 0.10) || !near(b.Price, 0) {
		t.Errorf("near breakdown = %+v", b)
	}
}

func TestBreakdownSumsToScoreAndScoreIsInUnitInterval(t *testing.T) {
	cs := []Candidate{cand("a", 10, 100), cand("b", 900, 0), cand("c", 450, 700)}
	cs[1].Freshness, cs[2].Trust = 1, 0
	for _, w := range []Weights{DefaultWeights, {Distance: 1}, {Price: 1}, {0.1, 0.2, 0.3, 0.4}} {
		for _, r := range mustRank(t, cs, w) {
			sum := r.Breakdown.Distance + r.Breakdown.Freshness + r.Breakdown.Trust + r.Breakdown.Price
			if !near(sum, r.Score) {
				t.Errorf("weights %+v: %s breakdown sums to %v, score %v", w, r.PharmacyID, sum, r.Score)
			}
			if r.Score < -eps || r.Score > 1+eps {
				t.Errorf("weights %+v: %s score %v outside 0..1", w, r.PharmacyID, r.Score)
			}
		}
	}
}

func TestRankedCarriesTheCandidate(t *testing.T) {
	c := Candidate{PharmacyID: "ph", ProductID: "pr-9", DistanceM: 42, Freshness: 0.3, Trust: 0.7, PriceChhertum: 123, Quantity: 4}
	got := mustRank(t, []Candidate{c}, DefaultWeights)
	if got[0].Candidate != c {
		t.Errorf("Candidate = %+v, want %+v", got[0].Candidate, c)
	}
}

// --- Ordering and ties ------------------------------------------------------------

func TestTiesAreBrokenByDistanceThenPharmacyThenProduct(t *testing.T) {
	// Only freshness counts, and all freshness is equal: every score ties.
	w := Weights{Freshness: 1}
	in := []Candidate{
		{PharmacyID: "b", ProductID: "p2", DistanceM: 100, Freshness: 0.5, Quantity: 1},
		{PharmacyID: "z", ProductID: "p1", DistanceM: 50, Freshness: 0.5, Quantity: 1},
		{PharmacyID: "b", ProductID: "p1", DistanceM: 100, Freshness: 0.5, Quantity: 1},
		{PharmacyID: "a", ProductID: "p1", DistanceM: 100, Freshness: 0.5, Quantity: 1},
	}
	got := mustRank(t, in, w)
	var order []string
	for _, r := range got {
		order = append(order, r.PharmacyID+"/"+r.ProductID)
	}
	want := []string{"z/p1", "a/p1", "b/p1", "b/p2"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestOrderDoesNotDependOnInputOrder(t *testing.T) {
	a := []Candidate{cand("a", 300, 500), cand("b", 100, 900), cand("c", 300, 500), cand("d", 200, 0)}
	b := []Candidate{a[3], a[1], a[2], a[0]}
	if !reflect.DeepEqual(ids(mustRank(t, a, DefaultWeights)), ids(mustRank(t, b, DefaultWeights))) {
		t.Error("the same candidates in a different order were ranked differently")
	}
}

func TestInputIsNotModified(t *testing.T) {
	in := []Candidate{cand("b", 900, 100), cand("a", 100, 900)}
	_ = mustRank(t, in, DefaultWeights)
	if in[0].PharmacyID != "b" {
		t.Error("Rank reordered the caller's slice")
	}
}

// No paid placement: nothing outside the four factors can change the order.
// Changing a factor moves a pharmacy only in the expected direction.
func TestImprovingAFactorNeverLowersRank(t *testing.T) {
	base := []Candidate{cand("x", 800, 600), cand("y", 400, 500), cand("z", 600, 700)}
	position := func(cs []Candidate, id string) int {
		for i, r := range mustRank(t, cs, DefaultWeights) {
			if r.PharmacyID == id {
				return i
			}
		}
		return -1
	}
	before := position(base, "x")
	better := map[string]func(*Candidate){
		"closer":       func(c *Candidate) { c.DistanceM = 100 },
		"fresher":      func(c *Candidate) { c.Freshness = 1 },
		"more trusted": func(c *Candidate) { c.Trust = 1 },
		"cheaper":      func(c *Candidate) { c.PriceChhertum = 100 },
	}
	for name, improve := range better {
		cs := append([]Candidate(nil), base...)
		improve(&cs[0])
		if after := position(cs, "x"); after > before {
			t.Errorf("%s: x moved from %d down to %d", name, before, after)
		}
	}
}

// --- median -----------------------------------------------------------------------

func TestMedian(t *testing.T) {
	cases := []struct {
		in   []float64
		want float64
	}{
		{nil, 0},
		{[]float64{7}, 7},
		{[]float64{3, 1, 2}, 2},
		{[]float64{4, 1, 3, 2}, 2.5},
	}
	for _, tc := range cases {
		if got := median(tc.in); !near(got, tc.want) {
			t.Errorf("median(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
