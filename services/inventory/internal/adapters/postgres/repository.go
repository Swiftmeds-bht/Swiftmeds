package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const stockColumns = `
	pharmacy_id,
	product_id,
	quantity_on_hand,
	quantity_reserved,
	confirmed_at
`

type database interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Repository stores inventory data in PostgreSQL.
type Repository struct {
	db database
}

// NewRepository creates a PostgreSQL inventory repository.
func NewRepository(db *pgxpool.Pool) (*Repository, error) {
	if db == nil {
		return nil, errors.New("inventory database is nil")
	}

	return &Repository{db: db}, nil
}

func scanStock(row pgx.Row) (domain.Stock, error) {
	var pharmacyID, productID string
	var onHand, reserved int32
	var confirmedAt time.Time

	if err := row.Scan(
		&pharmacyID,
		&productID,
		&onHand,
		&reserved,
		&confirmedAt,
	); err != nil {
		return domain.Stock{}, err
	}

	stock, err := domain.NewStock(
		pharmacyID,
		productID,
		onHand,
		reserved,
		confirmedAt,
	)
	if err != nil {
		return domain.Stock{}, fmt.Errorf("invalid stock row: %w", err)
	}

	return stock, nil
}

// Get returns stock for one pharmacy/product pair.
func (r *Repository) Get(
	ctx context.Context,
	key ports.StockKey,
) (domain.Stock, bool, error) {
	if strings.TrimSpace(key.PharmacyID) == "" ||
		strings.TrimSpace(key.ProductID) == "" {
		return domain.Stock{}, false, domain.ErrInvalidStockKey
	}

	query := `SELECT ` + stockColumns + `
		FROM inventory_stock
		WHERE pharmacy_id = $1 AND product_id = $2`

	stock, err := scanStock(r.db.QueryRow(
		ctx,
		query,
		key.PharmacyID,
		key.ProductID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Stock{}, false, nil
	}
	if err != nil {
		return domain.Stock{}, false, fmt.Errorf("get inventory stock: %w", err)
	}

	return stock, true, nil
}

// GetMany returns stock matching the supplied pharmacy and product IDs.
// When onlyAvailable is true, rows with no available units are omitted.
func (r *Repository) GetMany(
	ctx context.Context,
	pharmacyIDs, productIDs []string,
	onlyAvailable bool,
) ([]domain.Stock, error) {
	if len(pharmacyIDs) == 0 || len(productIDs) == 0 {
		return []domain.Stock{}, nil
	}

	query := `SELECT ` + stockColumns + `
		FROM inventory_stock
		WHERE pharmacy_id = ANY($1::text[])
			AND product_id = ANY($2::text[])
			AND (NOT $3 OR quantity_on_hand > quantity_reserved)
		ORDER BY product_id, pharmacy_id`

	rows, err := r.db.Query(ctx, query, pharmacyIDs, productIDs, onlyAvailable)
	if err != nil {
		return nil, fmt.Errorf("query inventory stock: %w", err)
	}
	defer rows.Close()

	stocks := make([]domain.Stock, 0)
	for rows.Next() {
		stock, err := scanStock(rows)
		if err != nil {
			return nil, fmt.Errorf("scan inventory stock: %w", err)
		}
		stocks = append(stocks, stock)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inventory stock: %w", err)
	}

	return stocks, nil
}

// ListByPharmacy returns a page of stock ordered by product ID.
func (r *Repository) ListByPharmacy(
	ctx context.Context,
	pharmacyID string,
	pageSize int32,
	pageToken string,
) ([]domain.Stock, string, error) {
	if strings.TrimSpace(pharmacyID) == "" {
		return nil, "", domain.ErrInvalidStockKey
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	afterProductID, err := decodePageToken(pageToken)
	if err != nil {
		return nil, "", err
	}

	query := `SELECT ` + stockColumns + `
		FROM inventory_stock
		WHERE pharmacy_id = $1 AND product_id > $2
		ORDER BY product_id
		LIMIT $3`

	rows, err := r.db.Query(ctx, query, pharmacyID, afterProductID, pageSize+1)
	if err != nil {
		return nil, "", fmt.Errorf("list pharmacy inventory: %w", err)
	}
	defer rows.Close()

	stocks := make([]domain.Stock, 0, pageSize)
	for rows.Next() {
		stock, err := scanStock(rows)
		if err != nil {
			return nil, "", fmt.Errorf("scan pharmacy inventory: %w", err)
		}
		stocks = append(stocks, stock)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate pharmacy inventory: %w", err)
	}

	var nextPageToken string
	if len(stocks) > int(pageSize) {
		stocks = stocks[:int(pageSize)]
		nextPageToken = encodePageToken(stocks[len(stocks)-1].ProductID)
	}

	return stocks, nextPageToken, nil
}

func encodePageToken(productID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(productID))
}

func decodePageToken(token string) (string, error) {
	if token == "" {
		return "", nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("invalid inventory page token: %w", err)
	}

	return string(decoded), nil
}
