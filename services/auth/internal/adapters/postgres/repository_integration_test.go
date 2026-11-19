//go:build integration

package postgres

import (
	"context"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
)

func TestRepositoryContract(t *testing.T) {
	url := os.Getenv("AUTH_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AUTH_TEST_DATABASE_URL to run Postgres repository tests")
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
	repotest.Run(t, func(t *testing.T) ports.UserRepository {
		if _, err := pool.Exec(ctx, "TRUNCATE refresh_tokens, users CASCADE"); err != nil {
			t.Fatal(err)
		}
		return NewRepository(pool)
	})
}
