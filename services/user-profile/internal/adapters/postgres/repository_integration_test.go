//go:build integration

package postgres

import (
	"context"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/user-profile/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
)

func TestRepositoryContract(t *testing.T) {
	url := os.Getenv("USER_PROFILE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set USER_PROFILE_TEST_DATABASE_URL to run Postgres repository tests")
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
	repotest.Run(t, func(t *testing.T) ports.ProfileRepository {
		if _, err := pool.Exec(ctx, "TRUNCATE user_profiles"); err != nil {
			t.Fatal(err)
		}
		return NewRepository(pool)
	})
}
