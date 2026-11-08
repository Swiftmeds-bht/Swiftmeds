// Package postgres is the FeedbackRepository backed by the feedback service's
// own database (feedback_db). Schema: services/feedback/migrations.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/ports"
)

var _ ports.FeedbackRepository = (*Repository)(nil)

// feedbackColumns is the column list of every SELECT, in the order scanFeedback reads it.
const feedbackColumns = `feedback_id, hold_id, pharmacy_id, product_id, was_in_stock, rating, comment, created_at`

// uniqueViolation is the Postgres error code for a unique index or key.
const uniqueViolation = "23505"

// holdIndex is the unique index that allows one feedback per hold (migration 001).
const holdIndex = "feedback_hold_uidx"

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Save inserts the feedback in a transaction and runs beforeCommit before the
// commit. If the hook fails, the transaction is rolled back, so nothing is
// stored. The unique index on hold_id is what refuses a second feedback for a
// hold, including two that arrive at the same moment: the second insert waits
// for the first transaction and then fails, before its hook runs.
func (r *Repository) Save(ctx context.Context, f domain.Feedback, beforeCommit func(context.Context) error) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO feedback (`+feedbackColumns+`)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			f.ID, f.HoldID, f.PharmacyID, f.ProductID, f.WasInStock, f.Rating, f.Comment, f.CreatedAt)
		if err != nil {
			if name, ok := violatedUnique(err); ok && name == holdIndex {
				return domain.ErrAlreadySubmitted
			}
			return fmt.Errorf("insert feedback: %w", err)
		}
		if beforeCommit != nil {
			return beforeCommit(ctx)
		}
		return nil
	})
}

// ListByPharmacy returns feedback newest first. The filter is built from
// placeholders only; no caller text is put into the SQL.
func (r *Repository) ListByPharmacy(ctx context.Context, f domain.ListFilter) ([]domain.Feedback, error) {
	args := []any{f.PharmacyID}
	query := `SELECT ` + feedbackColumns + ` FROM feedback WHERE pharmacy_id = $1`
	if f.After != nil {
		args = append(args, f.After.CreatedAt, f.After.ID)
		query += ` AND (created_at, feedback_id) < ($2::timestamptz, $3::text)`
	}
	query += ` ORDER BY created_at DESC, feedback_id DESC`
	if f.Limit > 0 {
		args = append(args, f.Limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list feedback: %w", err)
	}
	defer rows.Close()

	var out []domain.Feedback
	for rows.Next() {
		var item domain.Feedback
		err := rows.Scan(&item.ID, &item.HoldID, &item.PharmacyID, &item.ProductID,
			&item.WasInStock, &item.Rating, &item.Comment, &item.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan feedback: %w", err)
		}
		item.CreatedAt = item.CreatedAt.UTC()
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list feedback: %w", err)
	}
	return out, nil
}

// violatedUnique reports whether err is a unique violation, and which index or key.
func violatedUnique(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return pgErr.ConstraintName, true
	}
	return "", false
}
