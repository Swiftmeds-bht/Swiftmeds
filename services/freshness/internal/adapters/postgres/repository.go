// Package postgres is the outbound adapter that stores ratings in PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/ports"
)

var _ ports.RatingRepository = (*Repository)(nil)

const columns = "pharmacy_id, product_id, confirmed_at, confidence"

// Repository implements ports.RatingRepository on top of database/sql.
// The caller opens the *sql.DB with the pgx driver (github.com/jackc/pgx/v5/stdlib).
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Migrate runs every *.up.sql file in fsys, in name order. The files must be
// safe to run more than once, because the service runs them at every start.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	files, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	for _, name := range files {
		script, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := db.ExecContext(ctx, string(script)); err != nil {
			return fmt.Errorf("run migration %s: %w", name, err)
		}
	}
	return nil
}

// Save inserts the rating, or replaces the stored one if this confirmation is newer.
// If the stored confirmation is equal or newer, nothing changes and that row is returned.
func (r *Repository) Save(ctx context.Context, rating domain.Rating) (domain.Rating, error) {
	saved, err := scan(r.db.QueryRowContext(ctx, `
		INSERT INTO stock_confirmation (`+columns+`)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (pharmacy_id, product_id) DO UPDATE
		SET confirmed_at = EXCLUDED.confirmed_at, confidence = EXCLUDED.confidence
		WHERE EXCLUDED.confirmed_at > stock_confirmation.confirmed_at
		RETURNING `+columns,
		rating.PharmacyID, rating.ProductID, rating.ConfirmedAt.UTC(), string(rating.Confidence)))

	if errors.Is(err, sql.ErrNoRows) {
		// The WHERE above was false, so a newer confirmation is already stored.
		return scan(r.db.QueryRowContext(ctx,
			`SELECT `+columns+` FROM stock_confirmation WHERE pharmacy_id = $1 AND product_id = $2`,
			rating.PharmacyID, rating.ProductID))
	}
	return saved, err
}

// GetMany returns the ratings that exist for keys, in the order of keys. Missing keys are skipped.
func (r *Repository) GetMany(ctx context.Context, keys []domain.Key) ([]domain.Rating, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	pharmacies := make([]string, len(keys))
	products := make([]string, len(keys))
	for i, k := range keys {
		pharmacies[i], products[i] = k.PharmacyID, k.ProductID
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT s.pharmacy_id, s.product_id, s.confirmed_at, s.confidence
		FROM unnest($1::text[], $2::text[]) WITH ORDINALITY AS k(pharmacy_id, product_id, n)
		JOIN stock_confirmation s ON s.pharmacy_id = k.pharmacy_id AND s.product_id = k.product_id
		ORDER BY k.n`, pharmacies, products)
	return collect(rows, err)
}

// All returns every rating.
func (r *Repository) All(ctx context.Context) ([]domain.Rating, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+columns+` FROM stock_confirmation`)
	return collect(rows, err)
}

// UpdateConfidence changes the confidence of existing rows in one statement,
// so either every row changes or none does.
func (r *Repository) UpdateConfidence(ctx context.Context, ratings []domain.Rating) error {
	pharmacies := make([]string, len(ratings))
	products := make([]string, len(ratings))
	confidences := make([]string, len(ratings))
	for i, rt := range ratings {
		pharmacies[i], products[i], confidences[i] = rt.PharmacyID, rt.ProductID, string(rt.Confidence)
	}

	_, err := r.db.ExecContext(ctx, `
		UPDATE stock_confirmation s SET confidence = u.confidence
		FROM unnest($1::text[], $2::text[], $3::text[]) AS u(pharmacy_id, product_id, confidence)
		WHERE s.pharmacy_id = u.pharmacy_id AND s.product_id = u.product_id`,
		pharmacies, products, confidences)
	return err
}

// scanner is the part of *sql.Row and *sql.Rows that scan needs.
type scanner interface{ Scan(dest ...any) error }

func scan(s scanner) (domain.Rating, error) {
	var (
		rating     domain.Rating
		confidence string
	)
	if err := s.Scan(&rating.PharmacyID, &rating.ProductID, &rating.ConfirmedAt, &confidence); err != nil {
		return domain.Rating{}, err
	}
	rating.ConfirmedAt = rating.ConfirmedAt.UTC()
	rating.Confidence = domain.Confidence(confidence)
	return rating, nil
}

func collect(rows *sql.Rows, err error) ([]domain.Rating, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Rating
	for rows.Next() {
		rating, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rating)
	}
	return out, rows.Err()
}
