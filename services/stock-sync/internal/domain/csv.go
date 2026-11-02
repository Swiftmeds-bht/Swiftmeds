package domain

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxCSVBytes = 1 << 20

var (
	ErrEmptyCSV      = errors.New("CSV file is empty")
	ErrNoCSVRows     = errors.New("CSV file has no data rows")
	ErrCSVTooLarge   = errors.New("CSV file exceeds the 1 MB limit")
	ErrInvalidUTF8   = errors.New("CSV file is not valid UTF-8")
	ErrInvalidHeader = errors.New("CSV header must be product_id,quantity or product_id,quantity,price_chhertum")
	ErrMalformedCSV  = errors.New("CSV file is malformed")
)

type CSVRow struct {
	RowNumber     int32
	ProductID     string
	Quantity      int32
	PriceChhertum *int64
}

type RowError struct {
	RowNumber int32
	ProductID string
	Message   string
}

type ParsedCSV struct {
	Rows           []CSVRow
	Errors         []RowError
	RowsTotal      int32
	HasPriceColumn bool
}

// ParseCSV parses stock rows with an optional price_chhertum column.
// Row numbers count the header as row 1.
func ParseCSV(data []byte) (ParsedCSV, error) {
	if len(data) == 0 || strings.TrimSpace(string(data)) == "" {
		return ParsedCSV{}, ErrEmptyCSV
	}
	if len(data) > MaxCSVBytes {
		return ParsedCSV{}, ErrCSVTooLarge
	}
	if !utf8.Valid(data) {
		return ParsedCSV{}, ErrInvalidUTF8
	}

	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return ParsedCSV{}, ErrEmptyCSV
		}
		return ParsedCSV{}, fmt.Errorf("%w: %v", ErrMalformedCSV, err)
	}

	header[0] = strings.TrimPrefix(header[0], "\uFEFF")
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}

	hasPriceColumn := len(header) == 3 &&
		header[0] == "product_id" &&
		header[1] == "quantity" &&
		header[2] == "price_chhertum"

	if !hasPriceColumn &&
		(len(header) != 2 || header[0] != "product_id" || header[1] != "quantity") {
		return ParsedCSV{}, ErrInvalidHeader
	}

	parsed := ParsedCSV{
		Rows:           make([]CSVRow, 0),
		Errors:         make([]RowError, 0),
		HasPriceColumn: hasPriceColumn,
	}

	recordNumber := int32(1)
	seenProductIDs := make(map[string]struct{})

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ParsedCSV{}, fmt.Errorf("%w: %v", ErrMalformedCSV, err)
		}

		recordNumber++
		parsed.RowsTotal++

		row, rowErr := parseCSVRow(recordNumber, record, hasPriceColumn)
		if rowErr != nil {
			parsed.Errors = append(parsed.Errors, *rowErr)
			continue
		}

		key := strings.ToLower(row.ProductID)
		if _, exists := seenProductIDs[key]; exists {
			parsed.Errors = append(parsed.Errors, RowError{
				RowNumber: row.RowNumber,
				ProductID: row.ProductID,
				Message:   "duplicate product_id",
			})
			continue
		}
		seenProductIDs[key] = struct{}{}
		parsed.Rows = append(parsed.Rows, row)
	}

	if parsed.RowsTotal == 0 {
		return ParsedCSV{}, ErrNoCSVRows
	}
	return parsed, nil
}

func parseCSVRow(rowNumber int32, fields []string, hasPriceColumn bool) (CSVRow, *RowError) {
	row := CSVRow{RowNumber: rowNumber}
	fail := func(message string) (CSVRow, *RowError) {
		return CSVRow{}, &RowError{
			RowNumber: rowNumber,
			ProductID: row.ProductID,
			Message:   message,
		}
	}

	expectedFields := 2
	if hasPriceColumn {
		expectedFields = 3
	}
	if len(fields) != expectedFields {
		return fail(fmt.Sprintf("expected %d columns, got %d", expectedFields, len(fields)))
	}

	row.ProductID = strings.TrimSpace(fields[0])
	if row.ProductID == "" {
		return fail("product_id is required")
	}

	quantity, err := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 32)
	if err != nil {
		return fail("quantity must be a whole number")
	}
	if quantity < 0 {
		return fail("quantity must not be negative")
	}
	row.Quantity = int32(quantity)

	if hasPriceColumn {
		priceText := strings.TrimSpace(fields[2])
		if priceText != "" {
			price, err := strconv.ParseInt(priceText, 10, 64)
			if err != nil || price <= 0 {
				return fail("price_chhertum must be a positive whole number")
			}
			row.PriceChhertum = &price
		}
	}

	return row, nil
}
