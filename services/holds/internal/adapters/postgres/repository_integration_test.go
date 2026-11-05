//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/repotest"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/migrations"
)

// Run against a throwaway database (the tests empty the holds table):
//
//	HOLDS_TEST_DATABASE_URL=postgres://swiftmed:change-me@localhost:5432/holds_test?sslmode=disable \
//	  go test -tags integration ./internal/adapters/postgres/
func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("HOLDS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set HOLDS_TEST_DATABASE_URL to run the Postgres repository tests")
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
	repotest.Run(t, func(t *testing.T) ports.HoldRepository {
		if _, err := pool.Exec(context.Background(), `TRUNCATE holds`); err != nil {
			t.Fatal(err)
		}
		return NewRepository(pool)
	})
}

// The CHECK constraints and the unique index back up the domain rules, so a
// bug in the app layer cannot store a hold the rules forbid.
func TestDatabaseConstraintsBackUpTheDomainRules(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE holds`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	insert := func(id, pharmacy, status, code string, quantity int, expiresAt any) error {
		_, err := pool.Exec(ctx, `INSERT INTO holds (hold_id, user_id, pharmacy_id, product_id, quantity, status,
			pickup_code, created_at, expires_at, status_changed_at, status_reason)
			VALUES ($1, 'u-1', $2, 'med-1', $3, $4, $5, $6, $7, $6, '')`,
			id, pharmacy, quantity, status, code, now, expiresAt)
		return err
	}
	later := now.Add(time.Hour)

	if err := insert("h-1", "ph-1", "ACTIVE", "ABC234", 2, later); err != nil {
		t.Fatalf("a valid ACTIVE hold was refused: %v", err)
	}
	cases := []struct {
		name string
		err  error
	}{
		{"quantity 0", insert("h-2", "ph-1", "PENDING", "", 0, nil)},
		{"quantity 6", insert("h-3", "ph-1", "PENDING", "", 6, nil)},
		{"an unknown status", insert("h-4", "ph-1", "DONE", "", 1, nil)},
		{"an ACTIVE hold without a pickup code", insert("h-5", "ph-1", "ACTIVE", "", 1, later)},
		{"an ACTIVE hold without an expiry time", insert("h-6", "ph-1", "ACTIVE", "XYZ789", 1, nil)},
		{"the same code on two ACTIVE holds of one pharmacy", insert("h-7", "ph-1", "ACTIVE", "ABC234", 1, later)},
	}
	for _, c := range cases {
		if c.err == nil {
			t.Errorf("%s was stored, want the database to refuse it", c.name)
		}
	}
	if err := insert("h-8", "ph-2", "ACTIVE", "ABC234", 1, later); err != nil {
		t.Errorf("the same code at another pharmacy was refused: %v", err)
	}
	if err := insert("h-9", "ph-1", "COLLECTED", "ABC234", 1, nil); err != nil {
		t.Errorf("a COLLECTED hold with an old code was refused: %v", err)
	}
}

func TestCreateWithAnActiveCodeThatIsTakenIsAClash(t *testing.T) {
	pool := openTestDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE holds`); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(pool)
	now := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	active := func(id string) domain.Hold {
		h, err := domain.NewHold(id, "u-1", "ph-1", "med-1", 1, now)
		if err != nil {
			t.Fatal(err)
		}
		h, err = h.Reserve(now, time.Hour, "ABC234")
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if err := repo.Create(ctx, active("h-1"), 0); err != nil {
		t.Fatal(err)
	}

	err := repo.Create(ctx, active("h-2"), 0)

	if !errors.Is(err, domain.ErrPickupCodeTaken) {
		t.Fatalf("err = %v, want ErrPickupCodeTaken", err)
	}
}
