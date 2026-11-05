// Package postgres is the PriceRepository backed by the pricing service's own
// database (pricing_db). Schema: services/pricing/migrations.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/ports"
)

var _ ports.PriceRepository = (*Repository)(nil)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// SaveAll upserts every price in one transaction: all are saved or none.
func (r *Repository) SaveAll(ctx context.Context, prices []domain.Price) error {
	if len(prices) == 0 {
		return nil
	}
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, p := range prices {
			batch.Queue(`INSERT INTO prices (pharmacy_id, product_id, amount_chhertum, updated_at)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (pharmacy_id, product_id)
				DO UPDATE SET amount_chhertum = EXCLUDED.amount_chhertum, updated_at = EXCLUDED.updated_at`,
				p.PharmacyID, p.ProductID, p.AmountChhertum, p.UpdatedAt)
		}
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return fmt.Errorf("save prices: %w", err)
		}
		return nil
	})
}

// GetMany looks up all keys in one query by joining against the key arrays.
func (r *Repository) GetMany(ctx context.Context, keys []domain.Key) ([]domain.Price, error) {
	out := make([]domain.Price, 0, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	pharmacies := make([]string, len(keys))
	products := make([]string, len(keys))
	for i, k := range keys {
		pharmacies[i], products[i] = k.PharmacyID, k.ProductID
	}

	rows, err := r.db.Query(ctx, `SELECT p.pharmacy_id, p.product_id, p.amount_chhertum, p.updated_at
		FROM prices p
		JOIN (SELECT DISTINCT * FROM unnest($1::text[], $2::text[]) AS k(pharmacy_id, product_id)) k
		  ON p.pharmacy_id = k.pharmacy_id AND p.product_id = k.product_id`,
		pharmacies, products)
	if err != nil {
		return nil, fmt.Errorf("get prices: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p domain.Price
		if err := rows.Scan(&p.PharmacyID, &p.ProductID, &p.AmountChhertum, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan price: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get prices: %w", err)
	}
	return out, nil
}
