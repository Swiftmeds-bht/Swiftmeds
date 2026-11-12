package messaging

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
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

// Deduper makes sure each event ID is handled successfully at most once, even
// when several consumers receive the same message at the same time.
//
// Claim checks and reserves the event ID in one atomic step:
//
//   - claimed == false: the event was already handled; acknowledge it and do
//     not run the handler.
//   - claimed == true: this caller owns the event ID until it calls exactly one
//     of Claim.Commit (handled, never run it again) or Claim.Release (not
//     handled, a redelivery may claim it again). Another caller that claims the
//     same ID in the meantime waits until the first one finishes.
//
// If the process dies while it holds a claim, the claim is released, so a
// message that RabbitMQ redelivers is not lost.
type Deduper interface {
	Claim(ctx context.Context, eventID string) (claim Claim, claimed bool, err error)
}

// Claim is an event ID reserved by Deduper.Claim. Only the first call to
// Commit or Release has an effect; later calls return nil.
type Claim interface {
	// Commit records the event as processed.
	Commit(ctx context.Context) error
	// Release gives the event ID back without recording it.
	Release(ctx context.Context) error
}

// PostgresDeduper stores processed event IDs in PostgreSQL.
//
// A claim is an open transaction that has inserted the event ID with
// INSERT ... ON CONFLICT DO NOTHING:
//
//   - zero affected rows means the ID is already committed: a duplicate;
//   - while the transaction is open, another insert of the same ID blocks on
//     the primary key until it ends;
//   - Commit commits the row; Release rolls it back, and so does Postgres if
//     the connection drops, so a crashed consumer never leaves a stale claim.
//
// Each claim holds one pool connection while its handler runs. Consumers handle
// one delivery at a time, so this is one connection per consumer.
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

// Claim reserves eventID. See Deduper.
func (d *PostgresDeduper) Claim(ctx context.Context, eventID string) (Claim, bool, error) {
	if eventID == "" {
		return nil, false, ErrEmptyEventID
	}

	// The transaction must outlive a cancelled consumer context: database/sql
	// rolls a transaction back when its context ends, which would undo the
	// record of a handler that has already succeeded. The insert itself still
	// honours ctx, so a claim waiting on another consumer can be cancelled.
	tx, err := d.db.BeginTx(context.WithoutCancel(ctx), nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin claim for event %q: %w", eventID, err)
	}
	result, err := tx.ExecContext(ctx,
		`INSERT INTO swiftmed_processed_events (event_id)
		 VALUES ($1)
		 ON CONFLICT (event_id) DO NOTHING`,
		eventID,
	)
	if err != nil {
		return nil, false, errors.Join(
			fmt.Errorf("claim event %q: %w", eventID, err),
			rollbackError(tx.Rollback()),
		)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, false, errors.Join(
			fmt.Errorf("claim event %q: %w", eventID, err),
			rollbackError(tx.Rollback()),
		)
	}
	if inserted == 0 {
		if err := tx.Rollback(); err != nil {
			return nil, false, fmt.Errorf("end duplicate claim for event %q: %w", eventID, err)
		}
		return nil, false, nil
	}
	return &postgresClaim{tx: tx, eventID: eventID}, true, nil
}

type postgresClaim struct {
	mu      sync.Mutex
	tx      *sql.Tx
	eventID string
	done    bool
}

func (c *postgresClaim) Commit(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return nil
	}
	c.done = true
	if err := c.tx.Commit(); err != nil {
		return fmt.Errorf("mark event %q processed: %w", c.eventID, err)
	}
	return nil
}

func (c *postgresClaim) Release(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return nil
	}
	c.done = true
	if err := c.tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("release event %q: %w", c.eventID, err)
	}
	return nil
}

func rollbackError(err error) error {
	if err == nil || errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	return fmt.Errorf("roll back claim: %w", err)
}

// MemoryDeduper is an in-process Deduper for tests and single-instance tools.
// It behaves like PostgresDeduper: Claim is atomic, and a second Claim of an
// ID that is being handled waits until the first claim is committed or released.
type MemoryDeduper struct {
	mu        sync.Mutex
	processed map[string]bool
	inFlight  map[string]chan struct{}
}

// NewMemoryDeduper returns an empty MemoryDeduper.
func NewMemoryDeduper() *MemoryDeduper {
	return &MemoryDeduper{
		processed: make(map[string]bool),
		inFlight:  make(map[string]chan struct{}),
	}
}

// Claim reserves eventID. See Deduper.
func (d *MemoryDeduper) Claim(ctx context.Context, eventID string) (Claim, bool, error) {
	if eventID == "" {
		return nil, false, ErrEmptyEventID
	}
	for {
		d.mu.Lock()
		if d.processed[eventID] {
			d.mu.Unlock()
			return nil, false, nil
		}
		wait, busy := d.inFlight[eventID]
		if !busy {
			finished := make(chan struct{})
			d.inFlight[eventID] = finished
			d.mu.Unlock()
			return &memoryClaim{deduper: d, eventID: eventID, finished: finished}, true, nil
		}
		d.mu.Unlock()

		select {
		case <-wait:
		case <-ctx.Done():
			return nil, false, fmt.Errorf("claim event %q: %w", eventID, ctx.Err())
		}
	}
}

// IsProcessed reports whether eventID has been committed. It is meant for test assertions.
func (d *MemoryDeduper) IsProcessed(eventID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.processed[eventID]
}

type memoryClaim struct {
	deduper  *MemoryDeduper
	eventID  string
	finished chan struct{}
	once     sync.Once
}

func (c *memoryClaim) Commit(context.Context) error {
	c.finish(true)
	return nil
}

func (c *memoryClaim) Release(context.Context) error {
	c.finish(false)
	return nil
}

func (c *memoryClaim) finish(processed bool) {
	c.once.Do(func() {
		d := c.deduper
		d.mu.Lock()
		if processed {
			d.processed[c.eventID] = true
		}
		delete(d.inFlight, c.eventID)
		d.mu.Unlock()
		close(c.finished)
	})
}
