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
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/migrations"
)

// Run against a throwaway database (the tests empty the tables):
//
//	WATCHLIST_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/watchlist_test?sslmode=disable \
//	  go test -tags integration ./internal/adapters/postgres/
func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("WATCHLIST_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set WATCHLIST_TEST_DATABASE_URL to run the Postgres repository tests")
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

func truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE alerts, subscriptions`); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryContract(t *testing.T) {
	pool := openTestDB(t)
	repotest.Run(t, func(t *testing.T) ports.SubscriptionRepository {
		truncate(t, pool)
		return NewRepository(pool)
	})
}

// The alert row is written with the notification, and deleting the
// subscription removes its alerts too.
func TestNotifySavesTheAlertAndDeleteCascades(t *testing.T) {
	pool := openTestDB(t)
	truncate(t, pool)
	ctx := context.Background()
	repo := NewRepository(pool)
	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

	s := domain.Subscription{ID: "sub-1", UserID: "user-1", ProductID: "pr-1", Location: domain.Point{Lat: 27.47, Lng: 89.64}, RadiusM: 5000, CreatedAt: at}
	if _, _, err := repo.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.Notify(ctx, domain.Alert{ID: "al-1", SubscriptionID: "sub-1", UserID: "user-1", ProductID: "pr-1", PharmacyID: "ph-1", QuantityAvailable: 3, OccurredAt: at}); err != nil || !ok {
		t.Fatalf("Notify = %v, %v", ok, err)
	}
	if n := count(t, pool, `SELECT count(*) FROM alerts WHERE alert_id = 'al-1' AND pharmacy_id = 'ph-1' AND quantity_available = 3`); n != 1 {
		t.Fatalf("alert rows = %d, want 1", n)
	}

	if err := repo.Delete(ctx, "sub-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, `SELECT count(*) FROM alerts`); n != 0 {
		t.Errorf("alerts left after deleting the subscription: %d", n)
	}
}

func TestConcurrentDuplicateSubscribesKeepOne(t *testing.T) {
	pool := openTestDB(t)
	truncate(t, pool)
	repo := NewRepository(pool)
	at := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

	const n = 10
	errs := make(chan error, n)
	for i := range n {
		go func() {
			s := domain.Subscription{ID: "sub-" + string(rune('a'+i)), UserID: "user-1", ProductID: "pr-1", Location: domain.Point{Lat: 27.47, Lng: 89.64}, RadiusM: 5000, CreatedAt: at}
			_, _, err := repo.Create(context.Background(), s)
			errs <- err
		}()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if got := count(t, pool, `SELECT count(*) FROM subscriptions WHERE user_id = 'user-1'`); got != 1 {
		t.Errorf("%d subscriptions after %d concurrent subscribes, want 1", got, n)
	}
}

func count(t *testing.T, pool *pgxpool.Pool, sql string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
