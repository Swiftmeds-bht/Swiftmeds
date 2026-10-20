package messaging

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrEmptyEventID = errors.New("event ID is empty")

// DedupSchema creates the table PostgresDeduper reads and writes.
const DedupSchema = `CREATE TABLE IF NOT EXISTS swiftmed_processed_events (
	event_id     TEXT PRIMARY KEY,
	processed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
)`

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// EnsureSchema creates the deduplication table in the consuming service's own
// database. Call it at startup, before NewPostgresDeduper. It is idempotent.
func EnsureSchema(ctx context.Context, db execer) error {
	if db == nil {
		return errors.New("database is nil")
	}
	if _, err := db.ExecContext(ctx, DedupSchema); err != nil {
		return fmt.Errorf("create deduplication table: %w", err)
	}
	return nil
}

// Deduper tracks event IDs that have been processed.
type Deduper interface {
	IsProcessed(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, eventID string) error
}

// PostgresDeduper stores processed event IDs in PostgreSQL.
type PostgresDeduper struct {
	db *sql.DB
}

// NewPostgresDeduper creates a deduper backed by PostgreSQL.
func NewPostgresDeduper(db *sql.DB) (*PostgresDeduper, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &PostgresDeduper{db: db}, nil
}

// IsProcessed reports whether this event ID is already in the deduplication table.
func (d *PostgresDeduper) IsProcessed(ctx context.Context, eventID string) (bool, error) {
	if eventID == "" {
		return false, ErrEmptyEventID
	}

	var processed bool
	err := d.db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM swiftmed_processed_events WHERE event_id = $1
		)`,
		eventID,
	).Scan(&processed)
	if err != nil {
		return false, fmt.Errorf("check processed event %q: %w", eventID, err)
	}
	return processed, nil
}

// MarkProcessed records the event ID. Repeated inserts are safe.
func (d *PostgresDeduper) MarkProcessed(ctx context.Context, eventID string) error {
	if eventID == "" {
		return ErrEmptyEventID
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO swiftmed_processed_events (event_id)
		 VALUES ($1)
		 ON CONFLICT (event_id) DO NOTHING`,
		eventID,
	)
	if err != nil {
		return fmt.Errorf("mark event %q processed: %w", eventID, err)
	}
	return nil
}
