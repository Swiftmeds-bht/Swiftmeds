//go:build integration

package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/migrations"
)

// Run against a throwaway database (the tests empty the feedback table):
//
//	$env:FEEDBACK_TEST_DATABASE_URL="postgres://swiftmed:change-me@localhost:5432/feedback_test?sslmode=disable"
//	go test -tags integration ./internal/adapters/postgres/
func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("FEEDBACK_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set FEEDBACK_TEST_DATABASE_URL to run the Postgres repository tests")
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
	repotest.Run(t, func(t *testing.T) ports.FeedbackRepository {
		if _, err := pool.Exec(context.Background(), `TRUNCATE feedback`); err != nil {
			t.Fatal(err)
		}
		return NewRepository(pool)
	})
}

// The CHECK constraints and the unique index back up the domain rules, so a bug
// in the app layer cannot store feedback the rules forbid.
func TestDatabaseConstraintsBackUpTheDomainRules(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE feedback`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	insert := func(id, hold string, rating int, comment string) error {
		_, err := pool.Exec(ctx, `INSERT INTO feedback (feedback_id, hold_id, pharmacy_id, product_id,
			was_in_stock, rating, comment, created_at) VALUES ($1, $2, 'ph-1', 'med-1', true, $3, $4, $5)`,
			id, hold, rating, comment, now)
		return err
	}

	if err := insert("f-1", "h-1", 5, strings.Repeat("a", 500)); err != nil {
		t.Fatalf("valid feedback was refused: %v", err)
	}
	cases := []struct {
		name string
		err  error
	}{
		{"rating above 5", insert("f-2", "h-2", 6, "")},
		{"rating below 0", insert("f-3", "h-3", -1, "")},
		{"comment over 500 characters", insert("f-4", "h-4", 3, strings.Repeat("a", 501))},
		{"second feedback for a hold", insert("f-5", "h-1", 3, "")},
	}
	for _, tc := range cases {
		if tc.err == nil {
			t.Errorf("%s was stored, want the database to refuse it", tc.name)
		}
	}
}
