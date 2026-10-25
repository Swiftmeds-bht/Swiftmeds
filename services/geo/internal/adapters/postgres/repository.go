// Package postgres is the LocationRepository backed by geo_db.
// Schema: services/geo/migrations.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/ports"
)

var _ ports.LocationRepository = (*Repository)(nil)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Upsert(ctx context.Context, loc domain.Location) error {
	_, err := r.db.Exec(ctx, `INSERT INTO pharmacy_locations (pharmacy_id, latitude, longitude, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (pharmacy_id)
		DO UPDATE SET latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude, updated_at = now()`,
		loc.PharmacyID, loc.Point.Lat, loc.Point.Lng)
	if err != nil {
		return fmt.Errorf("upsert location: %w", err)
	}
	return nil
}

func (r *Repository) Remove(ctx context.Context, pharmacyID string) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM pharmacy_locations WHERE pharmacy_id = $1`, pharmacyID); err != nil {
		return fmt.Errorf("remove location: %w", err)
	}
	return nil
}

// WithinBox is the cheap pre-filter (uses the lat/lng index); the exact
// Haversine distance is computed in the domain.
func (r *Repository) WithinBox(ctx context.Context, b domain.Box) ([]domain.Location, error) {
	rows, err := r.db.Query(ctx, `SELECT pharmacy_id, latitude, longitude
		FROM pharmacy_locations
		WHERE latitude BETWEEN $1 AND $2 AND longitude BETWEEN $3 AND $4`,
		b.MinLat, b.MaxLat, b.MinLng, b.MaxLng)
	if err != nil {
		return nil, fmt.Errorf("query locations: %w", err)
	}
	defer rows.Close()

	var out []domain.Location
	for rows.Next() {
		var l domain.Location
		if err := rows.Scan(&l.PharmacyID, &l.Point.Lat, &l.Point.Lng); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query locations: %w", err)
	}
	return out, nil
}
