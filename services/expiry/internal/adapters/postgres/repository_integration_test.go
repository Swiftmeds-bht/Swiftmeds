//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/migrations"
)

// Run against a throwaway database (the tests empty the hold_deadlines table):
//
//	EXPIRY_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/expiry_test?sslmode=disable \
//	  go test -tags integration ./internal/adapters/postgres/
func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("EXPIRY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set EXPIRY_TEST_DATABASE_URL to run the Postgres repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if _, err := migrate.Up(ctx, sqlDB, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestRepositoryContract(t *testing.T) {
	pool := openTestDB(t)
	repotest.Run(t, func(t *testing.T) ports.DeadlineRepository {
		if _, err := pool.Exec(context.Background(), `TRUNCATE hold_deadlines`); err != nil {
			t.Fatal(err)
		}
		return NewRepository(pool)
	})
}

// The CHECK constraint backs up the rule "a deadline has an outcome exactly when it is settled".
func TestDatabaseRefusesAHalfSettledDeadline(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE hold_deadlines`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	_, errNoOutcome := pool.Exec(ctx,
		`INSERT INTO hold_deadlines (hold_id, expires_at, completed_at, outcome) VALUES ('h-1', $1, $1, '')`, now)
	_, errNoTime := pool.Exec(ctx,
		`INSERT INTO hold_deadlines (hold_id, expires_at, outcome) VALUES ('h-2', $1, 'EXPIRED')`, now)
	_, errBadOutcome := pool.Exec(ctx,
		`INSERT INTO hold_deadlines (hold_id, expires_at, completed_at, outcome) VALUES ('h-3', $1, $1, 'DONE')`, now)

	if errNoOutcome == nil || errNoTime == nil || errBadOutcome == nil {
		t.Errorf("errors = %v, %v, %v, want the database to refuse all three", errNoOutcome, errNoTime, errBadOutcome)
	}
}
