package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/ports"
)

var _ ports.UserRepository = (*Repository)(nil)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, user domain.User) error {
	_, err := r.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role, pharmacy_id, created_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)`,
		user.ID, user.Email, user.PasswordHash, user.Role, user.PharmacyID, user.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrUserExists
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.findUser(ctx, `SELECT id, email, password_hash, role, COALESCE(pharmacy_id, ''), created_at
		FROM users WHERE email = $1`, email)
}

func (r *Repository) FindUserByID(ctx context.Context, id string) (domain.User, error) {
	return r.findUser(ctx, `SELECT id, email, password_hash, role, COALESCE(pharmacy_id, ''), created_at
		FROM users WHERE id = $1`, id)
}

// LinkPharmacy sets the role and pharmacy in one statement, so two calls
// cannot link one account to two pharmacies.
func (r *Repository) LinkPharmacy(ctx context.Context, userID, pharmacyID string) (domain.User, error) {
	var user domain.User
	err := r.db.QueryRow(ctx, `UPDATE users SET role = 'PHARMACIST', pharmacy_id = $2
		WHERE id = $1 AND role <> 'ADMIN' AND (pharmacy_id IS NULL OR pharmacy_id = '' OR pharmacy_id = $2)
		RETURNING id, email, password_hash, role, COALESCE(pharmacy_id, ''), created_at`, userID, pharmacyID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.PharmacyID, &user.CreatedAt,
	)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, fmt.Errorf("link pharmacy: %w", err)
	}
	// Nothing updated: say why.
	existing, findErr := r.FindUserByID(ctx, userID)
	switch {
	case findErr != nil:
		return domain.User{}, findErr
	case existing.Role == domain.RoleAdmin:
		return domain.User{}, domain.ErrAdminLink
	default:
		return domain.User{}, domain.ErrAlreadyLinked
	}
}

func (r *Repository) findUser(ctx context.Context, query, value string) (domain.User, error) {
	var user domain.User
	err := r.db.QueryRow(ctx, query, value).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.PharmacyID, &user.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("find user: %w", err)
	}
	return user, nil
}

func (r *Repository) SaveRefreshToken(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `INSERT INTO refresh_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)`, tokenHash, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

func (r *Repository) ConsumeRefreshToken(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := r.db.QueryRow(ctx, `UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > NOW()
		RETURNING user_id`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrRefreshRevoked
	}
	if err != nil {
		return "", fmt.Errorf("consume refresh token: %w", err)
	}
	return userID, nil
}
