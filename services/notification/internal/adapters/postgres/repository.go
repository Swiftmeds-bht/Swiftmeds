package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/ports"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ ports.NotificationRepository = (*Repository)(nil)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, n domain.Notification) (bool, error) {
	params, err := json.Marshal(n.Params)
	if err != nil {
		return false, fmt.Errorf("encode notification params: %w", err)
	}
	tag, err := r.db.Exec(ctx, `INSERT INTO notifications
		(notification_id, event_id, user_id, channel, template, params, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (event_id) DO NOTHING`,
		n.ID, n.EventID, n.UserID, n.Channel, n.Template, params, n.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("insert notification: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) ListByUser(ctx context.Context, userID string, limit int, token string) ([]domain.Notification, string, error) {
	offset := 0
	if token != "" {
		offset, _ = strconv.Atoi(token)
		if offset < 0 {
			offset = 0
		}
	}
	rows, err := r.db.Query(ctx, `SELECT notification_id, event_id, user_id, channel, template, params, created_at, read_at
		FROM notifications WHERE user_id = $1 ORDER BY created_at DESC, notification_id DESC OFFSET $2 LIMIT $3`, userID, offset, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()
	out := make([]domain.Notification, 0, limit)
	for rows.Next() {
		var n domain.Notification
		var params []byte
		var readAt *time.Time
		if err := rows.Scan(&n.ID, &n.EventID, &n.UserID, &n.Channel, &n.Template, &params, &n.CreatedAt, &readAt); err != nil {
			return nil, "", fmt.Errorf("scan notification: %w", err)
		}
		if err := json.Unmarshal(params, &n.Params); err != nil {
			return nil, "", fmt.Errorf("decode notification params: %w", err)
		}
		if readAt != nil {
			n.ReadAt = *readAt
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = strconv.Itoa(offset + limit)
	}
	return out, next, nil
}
