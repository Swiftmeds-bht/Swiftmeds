package domain

import (
	"reflect"
	"testing"
)

func price(pharmacy, product string, amount int64) Price {
	return Price{Key: Key{PharmacyID: pharmacy, ProductID: product}, AmountChhertum: amount}
}

func keysOf(prices []Price) []Key {
	out := make([]Key, 0, len(prices))
	for _, p := range prices {
		out = append(out, p.Key)
	}
	return out
}

func TestSortCheapestFirst(t *testing.T) {
	cases := []struct {
		name string
		in   []Price
		want []Key
	}{
		{"empty", nil, []Key{}},
		{"single", []Price{price("ph-1", "pr-1", 500)}, []Key{{"ph-1", "pr-1"}}},
		{
			"cheapest first",
			[]Price{price("ph-1", "pr-1", 900), price("ph-2", "pr-1", 450), price("ph-3", "pr-1", 600)},
			[]Key{{"ph-2", "pr-1"}, {"ph-3", "pr-1"}, {"ph-1", "pr-1"}},
		},
		{
			"ties broken by pharmacy ID so the order is deterministic",
			[]Price{price("ph-c", "pr-1", 500), price("ph-a", "pr-1", 500), price("ph-b", "pr-1", 500)},
			[]Key{{"ph-a", "pr-1"}, {"ph-b", "pr-1"}, {"ph-c", "pr-1"}},
		},
		{
			"several products are grouped, each cheapest first",
			[]Price{price("ph-1", "pr-b", 100), price("ph-2", "pr-a", 300), price("ph-1", "pr-a", 200)},
			[]Key{{"ph-1", "pr-a"}, {"ph-2", "pr-a"}, {"ph-1", "pr-b"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SortCheapestFirst(tc.in)
			if !reflect.DeepEqual(keysOf(got), tc.want) {
				t.Errorf("order = %v, want %v", keysOf(got), tc.want)
			}
		})
	}
}

func TestSortCheapestFirstDoesNotModifyInput(t *testing.T) {
	in := []Price{price("ph-1", "pr-1", 900), price("ph-2", "pr-1", 450)}
	_ = SortCheapestFirst(in)
	if in[0].PharmacyID != "ph-1" {
		t.Error("input slice was reordered")
	}
}

func TestSortCheapestFirstIsStableAcrossInputOrders(t *testing.T) {
	a := []Price{price("ph-2", "pr-1", 500), price("ph-1", "pr-1", 500), price("ph-3", "pr-1", 100)}
	b := []Price{a[2], a[0], a[1]}
	if !reflect.DeepEqual(keysOf(SortCheapestFirst(a)), keysOf(SortCheapestFirst(b))) {
		t.Error("the same prices in a different order produced a different ranking")
	}
}
