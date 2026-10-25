//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/geo/migrations"
)

// Run against a throwaway database (the tests empty the table):
//
//	GEO_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/geo_test?sslmode=disable \
//	  go test -tags integration ./internal/adapters/postgres/
func TestRepositoryContract(t *testing.T) {
	url := os.Getenv("GEO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set GEO_TEST_DATABASE_URL to run the Postgres repository tests")
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

	repotest.Run(t, func(t *testing.T) ports.LocationRepository {
		if _, err := pool.Exec(ctx, `TRUNCATE pharmacy_locations`); err != nil {
			t.Fatal(err)
		}
		return NewRepository(pool)
	})
}
