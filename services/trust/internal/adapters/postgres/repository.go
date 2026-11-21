// Package postgres is the TrustRepository backed by the trust service's own
// database (trust_db). Schema: services/trust/migrations.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/ports"
)

var _ ports.TrustRepository = (*Repository)(nil)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const columns = `pharmacy_id, feedback_count, in_stock_count, weighted_in_stock, weighted_total, last_feedback_at`

// ApplyFeedback runs in one transaction:
//  1. remember the feedback ID; if it is already there, stop (counted before);
//  2. make sure the pharmacy has a row and lock it, so concurrent feedback for
//     the same pharmacy is applied one after the other, never lost;
//  3. save update(row).
func (r *Repository) ApplyFeedback(ctx context.Context, f domain.Feedback, update func(domain.Record) domain.Record) (bool, error) {
	applied := false
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO applied_feedback (feedback_id, pharmacy_id)
			VALUES ($1, $2) ON CONFLICT (feedback_id) DO NOTHING`, f.ID, f.PharmacyID)
		if err != nil {
			return fmt.Errorf("remember feedback: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `INSERT INTO pharmacy_trust (pharmacy_id)
			VALUES ($1) ON CONFLICT (pharmacy_id) DO NOTHING`, f.PharmacyID); err != nil {
			return fmt.Errorf("create trust record: %w", err)
		}
		current, err := scanOne(tx.QueryRow(ctx, `SELECT `+columns+` FROM pharmacy_trust
			WHERE pharmacy_id = $1 FOR UPDATE`, f.PharmacyID))
		if err != nil {
			return err
		}
		next := update(current)
		var lastAt *time.Time
		if !next.LastAt.IsZero() {
			lastAt = &next.LastAt
		}
		if _, err := tx.Exec(ctx, `UPDATE pharmacy_trust SET
				feedback_count = $2, in_stock_count = $3, weighted_in_stock = $4,
				weighted_total = $5, last_feedback_at = $6, updated_at = now()
			WHERE pharmacy_id = $1`,
			f.PharmacyID, next.FeedbackCount, next.InStockCount, next.WeightedInStock, next.WeightedTotal, lastAt); err != nil {
			return fmt.Errorf("save trust record: %w", err)
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

func (r *Repository) Records(ctx context.Context, pharmacyIDs []string) (map[string]domain.Record, error) {
	out := make(map[string]domain.Record, len(pharmacyIDs))
	if len(pharmacyIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `SELECT `+columns+` FROM pharmacy_trust WHERE pharmacy_id = ANY($1)`, pharmacyIDs)
	if err != nil {
		return nil, fmt.Errorf("query trust records: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		rec, err := scanOne(rows)
		if err != nil {
			return nil, err
		}
		out[rec.PharmacyID] = rec
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query trust records: %w", err)
	}
	return out, nil
}

func scanOne(row pgx.Row) (domain.Record, error) {
	var (
		rec    domain.Record
		lastAt *time.Time
	)
	if err := row.Scan(&rec.PharmacyID, &rec.FeedbackCount, &rec.InStockCount,
		&rec.WeightedInStock, &rec.WeightedTotal, &lastAt); err != nil {
		return domain.Record{}, fmt.Errorf("scan trust record: %w", err)
	}
	if lastAt != nil {
		rec.LastAt = lastAt.UTC()
	}
	return rec, nil
}
