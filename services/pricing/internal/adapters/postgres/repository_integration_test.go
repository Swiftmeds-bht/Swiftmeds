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
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/migrations"
)

// Run against a throwaway database (the tests empty the prices table):
//
//	PRICING_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/pricing_test?sslmode=disable \
//	  go test -tags integration ./internal/adapters/postgres/
func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("PRICING_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set PRICING_TEST_DATABASE_URL to run the Postgres repository tests")
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
	repotest.Run(t, func(t *testing.T) ports.PriceRepository {
		if _, err := pool.Exec(context.Background(), `TRUNCATE prices`); err != nil {
			t.Fatal(err)
		}
		return NewRepository(pool)
	})
}

// The CHECK constraint backs up the domain rule, and SaveAll is atomic.
func TestSaveAllIsAtomicAndRejectsNonPositiveAmounts(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE prices`); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	now := time.Now().UTC()

	err := repo.SaveAll(ctx, []domain.Price{
		{Key: domain.Key{PharmacyID: "ph-1", ProductID: "pr-1"}, AmountChhertum: 500, UpdatedAt: now},
		{Key: domain.Key{PharmacyID: "ph-1", ProductID: "pr-2"}, AmountChhertum: 0, UpdatedAt: now},
	})
	if err == nil {
		t.Fatal("SaveAll with a zero amount returned nil error")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM prices`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d rows saved, want 0 (the whole batch must roll back)", n)
	}
}
