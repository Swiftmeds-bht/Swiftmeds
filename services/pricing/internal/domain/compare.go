package domain

import (
	"cmp"
	"slices"
)

// SortCheapestFirst returns a copy of prices ordered for comparison: grouped by
// product, cheapest first, and equal prices ordered by pharmacy ID so the
// result never depends on database or map order. The input is not modified.
func SortCheapestFirst(prices []Price) []Price {
	out := slices.Clone(prices)
	if out == nil {
		out = []Price{}
	}
	slices.SortFunc(out, func(a, b Price) int {
		return cmp.Or(
			cmp.Compare(a.ProductID, b.ProductID),
			cmp.Compare(a.AmountChhertum, b.AmountChhertum),
			cmp.Compare(a.PharmacyID, b.PharmacyID),
		)
	})
	return out
}
