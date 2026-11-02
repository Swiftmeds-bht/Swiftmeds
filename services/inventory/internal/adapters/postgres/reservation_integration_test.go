//go:build integration

package postgres

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

var integrationSequence atomic.Uint64

func integrationDatabase(t *testing.T) (*Repository, *pgxpool.Pool) {
	t.Helper()

	url := os.Getenv("INVENTORY_TEST_DB_URL")
	if url == "" {
		t.Skip("set INVENTORY_TEST_DB_URL to run inventory integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}
	t.Cleanup(db.Close)

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	sqlDB := stdlib.OpenDBFromPool(db)
	_, migrationErr := migrate.Up(ctx, sqlDB, migrations.FS)
	closeErr := sqlDB.Close()
	if migrationErr != nil {
		t.Fatalf("apply inventory migrations: %v", migrationErr)
	}
	if closeErr != nil {
		t.Fatalf("close migration database: %v", closeErr)
	}

	repo, err := NewRepository(db)
	if err != nil {
		t.Fatalf("create inventory repository: %v", err)
	}
	return repo, db
}

func seedIntegrationStock(t *testing.T, db *pgxpool.Pool) (ports.StockKey, string) {
	t.Helper()

	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), integrationSequence.Add(1))
	key := ports.StockKey{
		PharmacyID: "scrum37-pharmacy-" + id,
		ProductID:  "scrum37-product-" + id,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := db.Exec(ctx, `
		INSERT INTO inventory_stock (
			pharmacy_id, product_id, quantity_on_hand, quantity_reserved, updated_by_user_id
		)
		VALUES ($1, $2, 1, 0, 'scrum-37-integration')
	`, key.PharmacyID, key.ProductID)
	if err != nil {
		t.Fatalf("seed test stock: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		if _, err := db.Exec(cleanupCtx, `
			DELETE FROM inventory_reservations
			WHERE pharmacy_id = $1 AND product_id = $2
		`, key.PharmacyID, key.ProductID); err != nil {
			t.Errorf("delete test reservations: %v", err)
		}

		if _, err := db.Exec(cleanupCtx, `
			DELETE FROM inventory_stock
			WHERE pharmacy_id = $1 AND product_id = $2
		`, key.PharmacyID, key.ProductID); err != nil {
			t.Errorf("delete test stock: %v", err)
		}
	})

	return key, id
}

func assertIntegrationStock(
	t *testing.T,
	db *pgxpool.Pool,
	key ports.StockKey,
	wantOnHand, wantReserved int32,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var onHand, reserved int32
	err := db.QueryRow(ctx, `
		SELECT quantity_on_hand, quantity_reserved
		FROM inventory_stock
		WHERE pharmacy_id = $1 AND product_id = $2
	`, key.PharmacyID, key.ProductID).Scan(&onHand, &reserved)
	if err != nil {
		t.Fatalf("read test stock: %v", err)
	}

	if onHand-reserved < 0 {
		t.Fatalf("available stock is negative: on_hand=%d reserved=%d", onHand, reserved)
	}
	if onHand != wantOnHand || reserved != wantReserved {
		t.Fatalf(
			"stock = on_hand %d, reserved %d; want on_hand %d, reserved %d",
			onHand, reserved, wantOnHand, wantReserved,
		)
	}
}

func TestConcurrentReserveAllowsOnlyOneWinnerForLastUnit(t *testing.T) {
	repo, db := integrationDatabase(t)
	key, testID := seedIntegrationStock(t, db)

	const workers = 50

	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	var winners atomic.Int32

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			stock, reserved, err := repo.Reserve(
				ctx,
				fmt.Sprintf("%s-last-unit-%d", testID, worker),
				key,
				1,
			)
			if err != nil {
				errs <- fmt.Errorf("worker %d reserve: %w", worker, err)
				return
			}
			if stock.QuantityAvailable() < 0 {
				errs <- fmt.Errorf("worker %d observed negative available stock", worker)
				return
			}
			if reserved {
				winners.Add(1)
			}
		}(i)
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
	if got := winners.Load(); got != 1 {
		t.Errorf("successful reservations = %d, want exactly 1", got)
	}

	assertIntegrationStock(t, db, key, 1, 1)
}

func TestConcurrentReserveAndReleaseKeepsStockValid(t *testing.T) {
	repo, db := integrationDatabase(t)
	key, testID := seedIntegrationStock(t, db)

	const (
		workers  = 50
		attempts = 5
	)

	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	var successes atomic.Int32

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start

			for attempt := 0; attempt < attempts; attempt++ {
				holdID := fmt.Sprintf("%s-mixed-%d-%d", testID, worker, attempt)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

				stock, reserved, err := repo.Reserve(ctx, holdID, key, 1)
				if err != nil {
					errs <- fmt.Errorf("worker %d attempt %d reserve: %w", worker, attempt, err)
					cancel()
					return
				}
				if stock.QuantityAvailable() < 0 {
					errs <- fmt.Errorf("worker %d attempt %d observed negative available stock", worker, attempt)
					cancel()
					return
				}

				if reserved {
					successes.Add(1)

					released, err := repo.ReleaseReservation(ctx, holdID)
					if err != nil {
						errs <- fmt.Errorf("worker %d attempt %d release: %w", worker, attempt, err)
						cancel()
						return
					}
					if released.QuantityAvailable() < 0 {
						errs <- fmt.Errorf("worker %d attempt %d release produced negative available stock", worker, attempt)
						cancel()
						return
					}
				}

				cancel()
			}
		}(worker)
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
	if got := successes.Load(); got == 0 {
		t.Error("no reservation succeeded during the reserve/release race")
	}

	assertIntegrationStock(t, db, key, 1, 0)
}
