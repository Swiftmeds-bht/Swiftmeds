package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseCSVAcceptsBOMSpacesAndBlankLines(t *testing.T) {
	bom := string(rune(0xFEFF)) // what Excel puts at the start of a UTF-8 CSV
	input := bom + " product_id , quantity \n\n  med-1 ,  4\r\nmed-2,0\n\n"

	got, err := ParseCSV([]byte(input))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if got.HasPriceColumn {
		t.Error("HasPriceColumn = true, want false")
	}
	if got.RowsTotal != 2 || len(got.Rows) != 2 || len(got.Errors) != 0 {
		t.Fatalf("parsed = %+v, want 2 clean rows", got)
	}
	if r := got.Rows[0]; r.ProductID != "med-1" || r.Quantity != 4 || r.RowNumber != 2 || r.PriceChhertum != nil {
		t.Errorf("row 0 = %+v, want med-1, quantity 4, row 2, no price", r)
	}
	if r := got.Rows[1]; r.ProductID != "med-2" || r.Quantity != 0 || r.RowNumber != 3 {
		t.Errorf("row 1 = %+v, want med-2, quantity 0, row 3", r)
	}
}

func TestParseCSVPriceRules(t *testing.T) {
	tests := []struct {
		price     string
		wantPrice int64 // 0 means no price
		wantError bool
	}{
		{"1", 1, false},
		{"2500", 2500, false},
		{" 42 ", 42, false},
		{"", 0, false},
		{"0", 0, true},
		{"-1", 0, true},
		{"12.50", 0, true},
		{"Nu.100", 0, true},
		{"99999999999999999999", 0, true},
	}
	for _, tt := range tests {
		t.Run("price "+tt.price, func(t *testing.T) {
			got, err := ParseCSV([]byte("product_id,quantity,price_chhertum\nmed-1,2," + tt.price + "\n"))
			if err != nil {
				t.Fatalf("ParseCSV: %v", err)
			}
			if tt.wantError {
				if len(got.Errors) != 1 || got.Errors[0].Message != "price_chhertum must be a positive whole number" ||
					got.Errors[0].ProductID != "med-1" || got.Errors[0].RowNumber != 2 {
					t.Fatalf("errors = %+v, want one price error for med-1 on row 2", got.Errors)
				}
				if len(got.Rows) != 0 {
					t.Fatalf("rows = %+v, want none", got.Rows)
				}
				return
			}
			if len(got.Rows) != 1 {
				t.Fatalf("rows = %+v errors = %+v, want one row", got.Rows, got.Errors)
			}
			p := got.Rows[0].PriceChhertum
			switch {
			case tt.wantPrice == 0 && p != nil:
				t.Fatalf("price = %d, want none", *p)
			case tt.wantPrice != 0 && (p == nil || *p != tt.wantPrice):
				t.Fatalf("price = %v, want %d", p, tt.wantPrice)
			}
		})
	}
}

func TestParseCSVQuantityRules(t *testing.T) {
	tests := []struct {
		quantity string
		want     int32
		wantErr  string
	}{
		{"0", 0, ""},
		{"1", 1, ""},
		{"2147483647", 2147483647, ""},
		{"2147483648", 0, "quantity must be a whole number"},
		{"-1", 0, "quantity must not be negative"},
		{"1.5", 0, "quantity must be a whole number"},
		{"", 0, "quantity must be a whole number"},
	}
	for _, tt := range tests {
		t.Run("quantity "+tt.quantity, func(t *testing.T) {
			got, err := ParseCSV([]byte("product_id,quantity\nmed-1," + tt.quantity + "\n"))
			if err != nil {
				t.Fatalf("ParseCSV: %v", err)
			}
			if tt.wantErr != "" {
				if len(got.Errors) != 1 || got.Errors[0].Message != tt.wantErr || got.Errors[0].ProductID != "med-1" {
					t.Fatalf("errors = %+v, want %q", got.Errors, tt.wantErr)
				}
				return
			}
			if len(got.Rows) != 1 || got.Rows[0].Quantity != tt.want {
				t.Fatalf("rows = %+v, want quantity %d", got.Rows, tt.want)
			}
		})
	}
}

func TestParseCSVColumnCountMustMatchTheHeader(t *testing.T) {
	got, err := ParseCSV([]byte("product_id,quantity,price_chhertum\nmed-1,2\nmed-2,3,100,extra\nmed-3,1,5\n"))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if got.RowsTotal != 3 || len(got.Rows) != 1 || got.Rows[0].ProductID != "med-3" {
		t.Fatalf("parsed = %+v, want only med-3", got)
	}
	want := []RowError{
		{RowNumber: 2, Message: "expected 3 columns, got 2"},
		{RowNumber: 3, Message: "expected 3 columns, got 4"},
	}
	if len(got.Errors) != len(want) {
		t.Fatalf("errors = %+v, want %+v", got.Errors, want)
	}
	for i := range want {
		if got.Errors[i] != want[i] {
			t.Errorf("error %d = %+v, want %+v", i, got.Errors[i], want[i])
		}
	}
}

func TestParseCSVRejectsHeaderVariants(t *testing.T) {
	for _, header := range []string{
		"quantity,product_id",
		"product_id",
		"product_id,quantity,price",
		"product_id,quantity,price_chhertum,notes",
		"PRODUCT_ID,QUANTITY",
	} {
		_, err := ParseCSV([]byte(header + "\nmed-1,1\n"))
		if !errors.Is(err, ErrInvalidHeader) {
			t.Errorf("header %q: error = %v, want ErrInvalidHeader", header, err)
		}
	}
}

func TestParseCSVFileLevelErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  error
	}{
		{"only whitespace", " \n\t\n", ErrEmptyCSV},
		{"header with price column only", "product_id,quantity,price_chhertum\n", ErrNoCSVRows},
		{"bare quote in header", "product_id,\"quantity\n", ErrMalformedCSV},
		{"bare quote in a row", "product_id,quantity\nmed-1,\"4\n", ErrMalformedCSV},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCSV([]byte(tt.input))
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if got.RowsTotal != 0 || got.Rows != nil {
				t.Fatalf("parsed = %+v, want the zero value on a file-level error", got)
			}
		})
	}
}

// A file of exactly MaxCSVBytes is allowed; one byte more is not.
func TestParseCSVSizeLimitBoundary(t *testing.T) {
	body := "product_id,quantity\nmed-1,1\n"
	exact := body + strings.Repeat("\n", MaxCSVBytes-len(body))
	if len(exact) != MaxCSVBytes {
		t.Fatalf("test setup: %d bytes", len(exact))
	}
	got, err := ParseCSV([]byte(exact))
	if err != nil {
		t.Fatalf("file of exactly %d bytes: %v", MaxCSVBytes, err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %+v, want med-1", got.Rows)
	}
	if _, err := ParseCSV([]byte(exact + "\n")); !errors.Is(err, ErrCSVTooLarge) {
		t.Fatalf("file of %d bytes: error = %v, want ErrCSVTooLarge", MaxCSVBytes+1, err)
	}
}

// Rows with errors still count towards RowsTotal and keep their row number, and
// a later duplicate of a rejected row is not reported as a duplicate.
func TestParseCSVRowNumbersAndDuplicatesAfterErrors(t *testing.T) {
	got, err := ParseCSV([]byte("product_id,quantity\nmed-1,x\nmed-1,2\nmed-1,3\n"))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if got.RowsTotal != 3 {
		t.Fatalf("RowsTotal = %d, want 3", got.RowsTotal)
	}
	if len(got.Rows) != 1 || got.Rows[0].RowNumber != 3 || got.Rows[0].Quantity != 2 {
		t.Fatalf("rows = %+v, want med-1 from row 3", got.Rows)
	}
	if len(got.Errors) != 2 ||
		got.Errors[0].RowNumber != 2 || got.Errors[0].Message != "quantity must be a whole number" ||
		got.Errors[1].RowNumber != 4 || got.Errors[1].Message != "duplicate product_id" {
		t.Fatalf("errors = %+v", got.Errors)
	}
}
