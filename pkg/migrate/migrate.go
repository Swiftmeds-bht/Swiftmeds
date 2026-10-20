// Package migrate applies a service's SQL migrations to its own database at startup.
//
// Each service embeds its migrations directory and calls Up before serving:
//
//	//go:embed *.sql
//	var FS embed.FS                      // in services/<name>/migrations
//
//	applied, err := migrate.Up(ctx, db, migrations.FS)
//
// Only files named <version>.up.sql are applied, in lexical order (so use a
// zero-padded prefix: 001_, 002_, ...). Each runs once, in its own transaction,
// and is recorded in the schema_migrations table. *.down.sql files are ignored.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// lockKey is the Postgres advisory lock that serialises migrations, so two
// replicas starting at once never apply the same file twice.
const lockKey int64 = 7_316_190_431

const upSuffix = ".up.sql"

// Migration is one *.up.sql file.
type Migration struct {
	Version string // file name without ".up.sql", e.g. "001_create_auth_tables"
	SQL     string
}

// Up applies every migration in fsys that is not yet recorded in
// schema_migrations and returns the versions it applied, in order.
func Up(ctx context.Context, db *sql.DB, fsys fs.FS) ([]string, error) {
	migrations, err := Load(fsys)
	if err != nil {
		return nil, err
	}

	// Advisory locks belong to a session, so pin one connection for the whole run.
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: get connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return nil, fmt.Errorf("migrate: acquire lock: %w", err)
	}
	defer func() {
		// Use a fresh context: the caller's may already be cancelled.
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey)
	}()

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return nil, fmt.Errorf("migrate: create schema_migrations: %w", err)
	}

	done, err := appliedVersions(ctx, conn)
	if err != nil {
		return nil, err
	}

	var applied []string
	for _, m := range Pending(migrations, done) {
		if err := apply(ctx, conn, m); err != nil {
			return applied, err
		}
		applied = append(applied, m.Version)
	}
	return applied, nil
}

// Load reads every *.up.sql file in the root of fsys, sorted by version.
func Load(fsys fs.FS) ([]Migration, error) {
	names, err := fs.Glob(fsys, "*"+upSuffix)
	if err != nil {
		return nil, fmt.Errorf("migrate: list migrations: %w", err)
	}
	sort.Strings(names)

	migrations := make([]Migration, 0, len(names))
	for _, name := range names {
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", name, err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("migrate: %s is empty", name)
		}
		migrations = append(migrations, Migration{
			Version: strings.TrimSuffix(name, upSuffix),
			SQL:     string(body),
		})
	}
	return migrations, nil
}

// Pending returns the migrations whose version is not in done, keeping their order.
func Pending(migrations []Migration, done map[string]bool) []Migration {
	var out []Migration
	for _, m := range migrations {
		if !done[m.Version] {
			out = append(out, m)
		}
	}
	return out
}

func appliedVersions(ctx context.Context, conn *sql.Conn) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: read schema_migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	done := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("migrate: scan version: %w", err)
		}
		done[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: read schema_migrations: %w", err)
	}
	return done, nil
}

// apply runs one migration and records it in the same transaction, so a
// failing file leaves neither its changes nor a schema_migrations row behind.
func apply(ctx context.Context, conn *sql.Conn, m Migration) (err error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate %s: begin: %w", m.Version, err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, rollback(tx))
		}
	}()

	if _, err = tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("migrate %s: %w", m.Version, err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, m.Version); err != nil {
		return fmt.Errorf("migrate %s: record version: %w", m.Version, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("migrate %s: commit: %w", m.Version, err)
	}
	return nil
}

func rollback(tx *sql.Tx) error {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("rollback: %w", err)
	}
	return nil
}
