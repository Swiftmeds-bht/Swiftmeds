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
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/migrations"
)

// Run against a throwaway database (the tests empty the tables):
//
//	TRUST_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/trust_test?sslmode=disable \
//	  go test -tags integration ./internal/adapters/postgres/
func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TRUST_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TRUST_TEST_DATABASE_URL to run the Postgres repository tests")
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
	if _, err := pool.Exec(context.Background(), `TRUNCATE pharmacy_trust, applied_feedback`); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryContract(t *testing.T) {
	pool := openTestDB(t)
	repotest.Run(t, func(t *testing.T) ports.TrustRepository {
		truncate(t, pool)
		return NewRepository(pool)
	})
}

// The real scoring rules on the real table: the record that comes back gives
// the same score as the one computed in memory.
func TestScoreSurvivesTheDatabase(t *testing.T) {
	pool := openTestDB(t)
	truncate(t, pool)
	ctx := context.Background()
	repo := NewRepository(pool)
	p := domain.DefaultParams
	at := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

	want := domain.Record{PharmacyID: "ph-1"}
	for i, inStock := range []bool{true, false, false, true, false} {
		f := domain.Feedback{ID: "fb-" + string(rune('a'+i)), PharmacyID: "ph-1", WasInStock: inStock, OccurredAt: at.Add(time.Duration(i) * 24 * time.Hour)}
		want = want.Apply(f, p)
		if ok, err := repo.ApplyFeedback(ctx, f, func(r domain.Record) domain.Record { return r.Apply(f, p) }); err != nil || !ok {
			t.Fatalf("ApplyFeedback = %v, %v", ok, err)
		}
	}
	got, err := repo.Records(ctx, []string{"ph-1"})
	if err != nil {
		t.Fatal(err)
	}
	now := at.Add(10 * 24 * time.Hour)
	if g, w := got["ph-1"].Score(now, p), want.Score(now, p); g != w {
		t.Errorf("score from the database = %v, computed = %v", g, w)
	}
}

// A failing update rolls back: the feedback ID is not remembered, so the
// redelivered event is applied.
func TestFailedApplyRemembersNothing(t *testing.T) {
	pool := openTestDB(t)
	truncate(t, pool)
	ctx := context.Background()
	repo := NewRepository(pool)
	f := domain.Feedback{ID: "fb-1", PharmacyID: "ph-1", WasInStock: true, OccurredAt: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)}

	// in_stock_count > feedback_count breaks a CHECK constraint, so the UPDATE fails.
	if _, err := repo.ApplyFeedback(ctx, f, func(r domain.Record) domain.Record {
		r.InStockCount = 5
		return r
	}); err == nil {
		t.Fatal("want the CHECK constraint to reject the record")
	}
	if ok, err := repo.ApplyFeedback(ctx, f, func(r domain.Record) domain.Record { return r.Apply(f, domain.DefaultParams) }); err != nil || !ok {
		t.Fatalf("retry: ApplyFeedback = %v, %v; want applied", ok, err)
	}
}
