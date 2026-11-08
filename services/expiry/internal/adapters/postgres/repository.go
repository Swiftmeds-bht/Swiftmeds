// Package postgres is the DeadlineRepository backed by the expiry service's own
// database (expiry_db). Schema: services/expiry/migrations.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/ports"
)

var _ ports.DeadlineRepository = (*Repository)(nil)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Save inserts the deadline. A hold that is already stored, settled or not,
// keeps its row: a redelivered event does not change a deadline and cannot
// reopen one that was settled.
func (r *Repository) Save(ctx context.Context, d domain.Deadline) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO hold_deadlines (hold_id, expires_at) VALUES ($1, $2)
		 ON CONFLICT (hold_id) DO NOTHING`,
		d.HoldID, d.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert deadline: %w", err)
	}
	return nil
}

// ListDue returns pending deadlines with expires_at <= asOf, in the order
// (expires_at, hold_id), after the cursor when there is one.
func (r *Repository) ListDue(ctx context.Context, asOf time.Time, after *domain.Deadline, limit int) ([]domain.Deadline, error) {
	query := `SELECT hold_id, expires_at FROM hold_deadlines
		WHERE completed_at IS NULL AND expires_at <= $1`
	args := []any{asOf}
	if after != nil {
		args = append(args, after.ExpiresAt, after.HoldID)
		query += fmt.Sprintf(` AND (expires_at, hold_id) > ($%d::timestamptz, $%d::text)`, len(args)-1, len(args))
	}
	query += ` ORDER BY expires_at, hold_id`
	if limit > 0 {
		args = append(args, limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list due deadlines: %w", err)
	}
	defer rows.Close()

	var out []domain.Deadline
	for rows.Next() {
		var d domain.Deadline
		if err := rows.Scan(&d.HoldID, &d.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan deadline: %w", err)
		}
		d.ExpiresAt = d.ExpiresAt.UTC()
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list due deadlines: %w", err)
	}
	return out, nil
}

// Complete settles a pending deadline. The UPDATE matches only while the
// deadline is still pending, so of several callers exactly one changes the row
// and sees true.
func (r *Repository) Complete(ctx context.Context, holdID string, outcome domain.Outcome, at time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE hold_deadlines SET completed_at = $2, outcome = $3
		 WHERE hold_id = $1 AND completed_at IS NULL`,
		holdID, at, string(outcome))
	if err != nil {
		return false, fmt.Errorf("complete deadline: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
