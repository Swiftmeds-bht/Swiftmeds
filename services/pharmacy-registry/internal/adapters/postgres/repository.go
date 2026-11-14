package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/ports"
)

var _ ports.PharmacyRepository = (*Repository)(nil)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, p domain.Pharmacy) error {
	hours, err := json.Marshal(p.Hours)
	if err != nil {
		return fmt.Errorf("encode hours: %w", err)
	}
	_, err = r.db.Exec(ctx, `INSERT INTO pharmacies
		(pharmacy_id, owner_user_id, name, license_number, dzongkhag, latitude, longitude, hours, status, status_reason, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		p.ID, p.OwnerID, p.Name, p.LicenseNumber, p.Dzongkhag, p.Location.Latitude,
		p.Location.Longitude, hours, string(p.Status), p.StatusReason, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert pharmacy: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (domain.Pharmacy, error) {
	return r.scanOne(r.db.QueryRow(ctx, pharmacySelect+` WHERE pharmacy_id = $1`, id))
}

func (r *Repository) List(ctx context.Context, verifiedOnly bool) ([]domain.Pharmacy, error) {
	query := pharmacySelect
	if verifiedOnly {
		query += ` WHERE status = 'VERIFIED'`
	}
	query += ` ORDER BY pharmacy_id`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list pharmacies: %w", err)
	}
	defer rows.Close()
	var out []domain.Pharmacy
	for rows.Next() {
		p, err := scanPharmacy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pharmacies: %w", err)
	}
	return out, nil
}

func (r *Repository) SetStatus(ctx context.Context, id string, status domain.Status, reason string, now time.Time) (domain.Pharmacy, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Pharmacy{}, fmt.Errorf("begin status update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var p domain.Pharmacy
	if err := scanRow(tx.QueryRow(ctx, pharmacySelect+` WHERE pharmacy_id = $1 FOR UPDATE`, id), &p); err != nil {
		return domain.Pharmacy{}, err
	}
	next, err := p.ApplyStatus(status, reason, now)
	if err != nil {
		return domain.Pharmacy{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE pharmacies SET status=$1,status_reason=$2,verified_at=$3 WHERE pharmacy_id=$4`,
		string(next.Status), next.StatusReason, next.VerifiedAt, id); err != nil {
		return domain.Pharmacy{}, fmt.Errorf("update pharmacy status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Pharmacy{}, fmt.Errorf("commit status update: %w", err)
	}
	return next, nil
}

const pharmacySelect = `SELECT pharmacy_id, owner_user_id, name, license_number, dzongkhag, latitude, longitude, hours, status, status_reason, created_at, verified_at FROM pharmacies`

type rowScanner interface{ Scan(...any) error }

func (r *Repository) scanOne(row pgx.Row) (domain.Pharmacy, error) {
	var p domain.Pharmacy
	if err := scanRow(row, &p); err != nil {
		return domain.Pharmacy{}, err
	}
	return p, nil
}

func scanPharmacy(row rowScanner) (domain.Pharmacy, error) {
	var p domain.Pharmacy
	if err := scanRow(row, &p); err != nil {
		return domain.Pharmacy{}, err
	}
	return p, nil
}

func scanRow(row rowScanner, p *domain.Pharmacy) error {
	var hours []byte
	var status string
	if err := row.Scan(&p.ID, &p.OwnerID, &p.Name, &p.LicenseNumber, &p.Dzongkhag, &p.Location.Latitude,
		&p.Location.Longitude, &hours, &status, &p.StatusReason, &p.CreatedAt, &p.VerifiedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("scan pharmacy: %w", err)
	}
	if err := json.Unmarshal(hours, &p.Hours); err != nil {
		return fmt.Errorf("decode hours: %w", err)
	}
	p.Status = domain.Status(status)
	return nil
}
