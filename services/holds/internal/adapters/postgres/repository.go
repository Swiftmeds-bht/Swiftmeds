// Package postgres is the HoldRepository backed by the holds service's own
// database (holds_db). Schema: services/holds/migrations.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
)

var _ ports.HoldRepository = (*Repository)(nil)

// holdColumns is the column list of every SELECT, in the order scanHold reads it.
const holdColumns = `hold_id, user_id, pharmacy_id, product_id, quantity, status, pickup_code,
	created_at, expires_at, status_changed_at, status_reason`

// uniqueViolation is the Postgres error code for a unique index or key.
const uniqueViolation = "23505"

// activeCodeIndex is the unique index that keeps pickup codes unique among a
// pharmacy's ACTIVE holds (migration 001).
const activeCodeIndex = "holds_active_code_uidx"

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create inserts the hold. When maxOpen is above zero, the patient's open
// holds are counted in the same transaction, under a lock that is held for
// that patient only, so two creates for one patient cannot both pass the check.
func (r *Repository) Create(ctx context.Context, h domain.Hold, maxOpen int) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		if maxOpen > 0 {
			// Released at the end of the transaction. Other patients are not blocked.
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, h.UserID); err != nil {
				return fmt.Errorf("lock patient holds: %w", err)
			}
			var open int
			err := tx.QueryRow(ctx,
				`SELECT count(*) FROM holds WHERE user_id = $1 AND status IN ('PENDING', 'ACTIVE')`,
				h.UserID).Scan(&open)
			if err != nil {
				return fmt.Errorf("count open holds: %w", err)
			}
			if open >= maxOpen {
				return domain.ErrLimitReached
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO holds (`+holdColumns+`)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			h.ID, h.UserID, h.PharmacyID, h.ProductID, h.Quantity, string(h.Status), h.PickupCode,
			h.CreatedAt, nullTime(h.ExpiresAt), h.StatusChangedAt, h.StatusReason)
		if err != nil {
			if name, ok := violatedUnique(err); ok {
				if name == activeCodeIndex {
					return domain.ErrPickupCodeTaken
				}
				return domain.ErrAlreadyExists
			}
			return fmt.Errorf("insert hold: %w", err)
		}
		return nil
	})
}

func (r *Repository) Get(ctx context.Context, id string) (domain.Hold, error) {
	row := r.db.QueryRow(ctx, `SELECT `+holdColumns+` FROM holds WHERE hold_id = $1`, id)
	h, err := scanHold(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Hold{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Hold{}, fmt.Errorf("get hold: %w", err)
	}
	return h, nil
}

// Update changes the mutable columns, but only while the stored status is still
// from. That one condition is what makes Collect and Expire safe when they
// arrive at the same moment: the database lets exactly one UPDATE match.
func (r *Repository) Update(ctx context.Context, h domain.Hold, from domain.Status) error {
	tag, err := r.db.Exec(ctx, `UPDATE holds
		SET status = $1, pickup_code = $2, expires_at = $3, status_changed_at = $4, status_reason = $5
		WHERE hold_id = $6 AND status = $7`,
		string(h.Status), h.PickupCode, nullTime(h.ExpiresAt), h.StatusChangedAt, h.StatusReason,
		h.ID, string(from))
	if err != nil {
		if name, ok := violatedUnique(err); ok && name == activeCodeIndex {
			return domain.ErrPickupCodeTaken
		}
		return fmt.Errorf("update hold: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}

	// No row matched: either there is no such hold, or its status is not from.
	var one int
	err = r.db.QueryRow(ctx, `SELECT 1 FROM holds WHERE hold_id = $1`, h.ID).Scan(&one)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.ErrNotFound
	case err != nil:
		return fmt.Errorf("check hold: %w", err)
	default:
		return domain.ErrConflict
	}
}

// List returns holds newest first. The filter is built from placeholders only;
// no caller text is put into the SQL.
func (r *Repository) List(ctx context.Context, f domain.ListFilter) ([]domain.Hold, error) {
	var conditions []string
	var args []any
	if f.UserID != "" {
		args = append(args, f.UserID)
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if f.PharmacyID != "" {
		args = append(args, f.PharmacyID)
		conditions = append(conditions, fmt.Sprintf("pharmacy_id = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, string(f.Status))
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.After != nil {
		args = append(args, f.After.CreatedAt, f.After.ID)
		conditions = append(conditions,
			fmt.Sprintf("(created_at, hold_id) < ($%d::timestamptz, $%d::text)", len(args)-1, len(args)))
	}

	query := `SELECT ` + holdColumns + ` FROM holds`
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY created_at DESC, hold_id DESC`
	if f.Limit > 0 {
		args = append(args, f.Limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list holds: %w", err)
	}
	defer rows.Close()

	var out []domain.Hold
	for rows.Next() {
		h, err := scanHold(rows)
		if err != nil {
			return nil, fmt.Errorf("scan hold: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list holds: %w", err)
	}
	return out, nil
}

// scanner is the Scan method that pgx.Row and pgx.Rows share.
type scanner interface {
	Scan(dest ...any) error
}

func scanHold(s scanner) (domain.Hold, error) {
	var h domain.Hold
	var status string
	var expiresAt *time.Time
	err := s.Scan(&h.ID, &h.UserID, &h.PharmacyID, &h.ProductID, &h.Quantity, &status, &h.PickupCode,
		&h.CreatedAt, &expiresAt, &h.StatusChangedAt, &h.StatusReason)
	if err != nil {
		return domain.Hold{}, err
	}
	h.Status = domain.Status(status)
	h.CreatedAt = h.CreatedAt.UTC()
	h.StatusChangedAt = h.StatusChangedAt.UTC()
	if expiresAt != nil {
		h.ExpiresAt = expiresAt.UTC()
	}
	return h, nil
}

// nullTime stores the zero time (a hold that is not ACTIVE yet) as NULL.
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// violatedUnique reports whether err is a unique violation, and which index or key.
func violatedUnique(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return pgErr.ConstraintName, true
	}
	return "", false
}
