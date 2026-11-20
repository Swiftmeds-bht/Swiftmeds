//go:build integration

package postgres

import (
	"context"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pharmacy-registry/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
)

func TestRepositoryContract(t *testing.T) {
	url := os.Getenv("PHARMACY_REGISTRY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set PHARMACY_REGISTRY_TEST_DATABASE_URL to run Postgres repository tests")
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
	repotest.Run(t, func(t *testing.T) ports.PharmacyRepository {
		if _, err := pool.Exec(ctx, "TRUNCATE pharmacies"); err != nil {
			t.Fatal(err)
		}
		return NewRepository(pool)
	})
}
