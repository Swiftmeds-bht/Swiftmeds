package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/ports"
)

var _ ports.ProfileRepository = (*Repository)(nil)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, p domain.Profile) error {
	_, err := r.db.Exec(ctx, `INSERT INTO user_profiles
		(user_id, name, phone, home_location, preferred_language, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		p.UserID, p.Name, p.Phone, p.HomeLocation, p.PreferredLanguage, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrProfileExists
		}
		return fmt.Errorf("insert profile: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, userID string) (domain.Profile, error) {
	var p domain.Profile
	err := r.db.QueryRow(ctx, `SELECT user_id, name, phone, home_location, preferred_language, created_at, updated_at
		FROM user_profiles WHERE user_id = $1`, userID).Scan(
		&p.UserID, &p.Name, &p.Phone, &p.HomeLocation, &p.PreferredLanguage, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Profile{}, domain.ErrProfileNotFound
	}
	if err != nil {
		return domain.Profile{}, fmt.Errorf("select profile: %w", err)
	}
	return p, nil
}

func (r *Repository) Update(ctx context.Context, p domain.Profile) error {
	tag, err := r.db.Exec(ctx, `UPDATE user_profiles SET name = $2, phone = $3,
		home_location = $4, preferred_language = $5, updated_at = $6 WHERE user_id = $1`,
		p.UserID, p.Name, p.Phone, p.HomeLocation, p.PreferredLanguage, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrProfileNotFound
	}
	return nil
}
