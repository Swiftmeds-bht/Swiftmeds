package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseCSVWithOptionalPrices(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantPriceColumn bool
		wantPrice       bool
	}{
		{
			name:            "stock only",
			input:           "product_id,quantity\nmed-1,4\n",
			wantPriceColumn: false,
		},
		{
			name:            "stock with price",
			input:           "product_id,quantity,price_chhertum\nmed-1,4,2500\n",
			wantPriceColumn: true,
			wantPrice:       true,
		},
		{
			name:            "price column with blank price",
			input:           "product_id,quantity,price_chhertum\nmed-1,4,\n",
			wantPriceColumn: true,
			wantPrice:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseCSV([]byte(test.input))
			if err != nil {
				t.Fatalf("ParseCSV returned error: %v", err)
			}
			if got.RowsTotal != 1 || len(got.Rows) != 1 {
				t.Fatalf("parsed rows = %+v, total = %d", got.Rows, got.RowsTotal)
			}
			if got.HasPriceColumn != test.wantPriceColumn {
				t.Errorf("HasPriceColumn = %v, want %v", got.HasPriceColumn, test.wantPriceColumn)
			}
			if (got.Rows[0].PriceChhertum != nil) != test.wantPrice {
				t.Errorf("PriceChhertum = %v, want price present %v",
					got.Rows[0].PriceChhertum, test.wantPrice)
			}
		})
	}
}

func TestParseCSVCollectsInvalidRows(t *testing.T) {
	input := strings.Join([]string{
		"product_id,quantity",
		"good-med,3",
		"negative-med,-1",
		"bad-med,not-a-number",
		",5",
		"wide-row,2,extra",
	}, "\n")

	got, err := ParseCSV([]byte(input))
	if err != nil {
		t.Fatalf("ParseCSV returned error: %v", err)
	}
	if got.RowsTotal != 5 {
		t.Fatalf("RowsTotal = %d, want 5", got.RowsTotal)
	}
	if len(got.Rows) != 1 || got.Rows[0].ProductID != "good-med" {
		t.Fatalf("valid rows = %+v, want only good-med", got.Rows)
	}
	if len(got.Errors) != 4 {
		t.Fatalf("got %d row errors, want 4: %+v", len(got.Errors), got.Errors)
	}
	if got.Errors[0].RowNumber != 3 || got.Errors[0].Message != "quantity must not be negative" {
		t.Errorf("negative quantity error = %+v", got.Errors[0])
	}
}

func TestParseCSVRejectsInvalidFiles(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  error
	}{
		{name: "empty", input: nil, want: ErrEmptyCSV},
		{name: "header only", input: []byte("product_id,quantity\n"), want: ErrNoCSVRows},
		{name: "bad header", input: []byte("medicine,amount\nm1,2\n"), want: ErrInvalidHeader},
		{name: "invalid utf8", input: []byte{0xff, 0xfe}, want: ErrInvalidUTF8},
		{name: "too large", input: make([]byte, MaxCSVBytes+1), want: ErrCSVTooLarge},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseCSV(test.input)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestParseCSVDuplicateProductIsRejected(t *testing.T) {
	input := "product_id,quantity\nmed-1,3\nMED-1,5\n"

	got, err := ParseCSV([]byte(input))
	if err != nil {
		t.Fatalf("ParseCSV returned error: %v", err)
	}
	if len(got.Rows) != 1 || got.Rows[0].Quantity != 3 {
		t.Fatalf("valid rows = %+v, want first med-1 row only", got.Rows)
	}
	if len(got.Errors) != 1 || got.Errors[0].Message != "duplicate product_id" {
		t.Fatalf("row errors = %+v, want duplicate product_id", got.Errors)
	}
}
