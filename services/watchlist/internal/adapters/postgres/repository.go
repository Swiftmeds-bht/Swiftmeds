// Package postgres is the SubscriptionRepository backed by the watchlist
// service's own database (watchlist_db). Schema: services/watchlist/migrations.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/ports"
)

var _ ports.SubscriptionRepository = (*Repository)(nil)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const columns = `subscription_id, user_id, product_id, latitude, longitude, radius_m, created_at, notified_at`

// Create relies on the partial unique index on (user_id, product_id) for waiting
// subscriptions: if the insert conflicts, the existing one is returned. Two
// concurrent identical requests therefore still end with one subscription.
func (r *Repository) Create(ctx context.Context, s domain.Subscription) (domain.Subscription, bool, error) {
	tag, err := r.db.Exec(ctx, `INSERT INTO subscriptions
		(subscription_id, user_id, product_id, latitude, longitude, radius_m, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, product_id) WHERE notified_at IS NULL DO NOTHING`,
		s.ID, s.UserID, s.ProductID, s.Location.Lat, s.Location.Lng, s.RadiusM, s.CreatedAt)
	if err != nil {
		return domain.Subscription{}, false, fmt.Errorf("insert subscription: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return s, true, nil
	}
	existing, err := scanOne(r.db.QueryRow(ctx, `SELECT `+columns+` FROM subscriptions
		WHERE user_id = $1 AND product_id = $2 AND notified_at IS NULL`, s.UserID, s.ProductID))
	if err != nil {
		return domain.Subscription{}, false, fmt.Errorf("read existing subscription: %w", err)
	}
	return existing, false, nil
}

func (r *Repository) Delete(ctx context.Context, subscriptionID, userID string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM subscriptions WHERE subscription_id = $1 AND user_id = $2`, subscriptionID, userID)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) ListByUser(ctx context.Context, userID string) ([]domain.Subscription, error) {
	return r.query(ctx, `SELECT `+columns+` FROM subscriptions
		WHERE user_id = $1 ORDER BY created_at DESC, subscription_id`, userID)
}

func (r *Repository) ActiveForProduct(ctx context.Context, productID string) ([]domain.Subscription, error) {
	return r.query(ctx, `SELECT `+columns+` FROM subscriptions
		WHERE product_id = $1 AND notified_at IS NULL ORDER BY created_at DESC, subscription_id`, productID)
}

// Notify marks the subscription and saves the alert in one transaction. The
// "notified_at IS NULL" condition makes a second Notify for the same
// subscription change nothing, even when two consumers race.
func (r *Repository) Notify(ctx context.Context, a domain.Alert) (bool, error) {
	saved := false
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE subscriptions SET notified_at = $2
			WHERE subscription_id = $1 AND notified_at IS NULL`, a.SubscriptionID, a.OccurredAt)
		if err != nil {
			return fmt.Errorf("mark subscription notified: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `INSERT INTO alerts
			(alert_id, subscription_id, user_id, product_id, pharmacy_id, quantity_available, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			a.ID, a.SubscriptionID, a.UserID, a.ProductID, a.PharmacyID, a.QuantityAvailable, a.OccurredAt); err != nil {
			return fmt.Errorf("insert alert: %w", err)
		}
		saved = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return saved, nil
}

func (r *Repository) query(ctx context.Context, sql string, arg string) ([]domain.Subscription, error) {
	rows, err := r.db.Query(ctx, sql, arg)
	if err != nil {
		return nil, fmt.Errorf("query subscriptions: %w", err)
	}
	defer rows.Close()
	out := []domain.Subscription{}
	for rows.Next() {
		s, err := scanOne(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query subscriptions: %w", err)
	}
	return out, nil
}

func scanOne(row pgx.Row) (domain.Subscription, error) {
	var (
		s        domain.Subscription
		notified *time.Time
	)
	err := row.Scan(&s.ID, &s.UserID, &s.ProductID, &s.Location.Lat, &s.Location.Lng, &s.RadiusM, &s.CreatedAt, &notified)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("scan subscription: %w", err)
	}
	if notified != nil {
		s.NotifiedAt = *notified
	}
	return s, nil
}
