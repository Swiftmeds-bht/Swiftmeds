package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func transactionRepository(
	rows []pgx.Row,
	reservationInsertAffected int64,
	failExecAt int,
	execErr error,
) (*Repository, *fakeTx) {
	tx := &fakeTx{}
	var rowIndex int
	var execCount int

	tx.queryRowFn = func(context.Context, string, ...any) pgx.Row {
		if rowIndex >= len(rows) {
			return fakeRow{err: pgx.ErrNoRows}
		}
		row := rows[rowIndex]
		rowIndex++
		return row
	}

	tx.execFn = func(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
		execCount++
		if failExecAt == execCount {
			return pgconn.NewCommandTag(""), execErr
		}
		if strings.Contains(query, "INSERT INTO inventory_reservations") {
			if reservationInsertAffected == 0 {
				return pgconn.NewCommandTag("INSERT 0 0"), nil
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		}
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}

	db := &fakeDB{
		beginFn: func(context.Context) (pgx.Tx, error) {
			return tx, nil
		},
	}
	return &Repository{db: db}, tx
}

func reservationRow(pharmacy, product string, quantity int32, status string) pgx.Row {
	return fakeRow{values: []any{pharmacy, product, quantity, status}}
}

func stockTestRow(pharmacy, product string, onHand, reserved int32) pgx.Row {
	return fakeRow{values: stockValues(pharmacy, product, onHand, reserved)}
}

func validTestStock(t *testing.T, pharmacy, product string, onHand, reserved int32) domain.Stock {
	t.Helper()
	stock, err := domain.NewStock(pharmacy, product, onHand, reserved, time.Now())
	if err != nil {
		t.Fatalf("create stock: %v", err)
	}
	return stock
}

func TestSetStockValidationAndTransaction(t *testing.T) {
	repo := &Repository{}

	if _, _, err := repo.Set(context.Background(), domain.Stock{}, "user-1"); !errors.Is(err, domain.ErrInvalidStockKey) {
		t.Fatalf("Set invalid key error = %v", err)
	}

	negative := validTestStock(t, "pharmacy-1", "product-1", 0, 0)
	negative.QuantityOnHand = -1
	if _, _, err := repo.Set(context.Background(), negative, "user-1"); !errors.Is(err, domain.ErrNegativeStock) {
		t.Fatalf("Set negative quantity error = %v", err)
	}

	requested, err := domain.NewStock("pharmacy-1", "product-1", 12, 2, time.Now())
	if err != nil {
		t.Fatalf("create request stock: %v", err)
	}

	repo, tx := transactionRepository(
		[]pgx.Row{
			stockTestRow("pharmacy-1", "product-1", 10, 2),
			stockTestRow("pharmacy-1", "product-1", 12, 2),
		},
		1,
		0,
		nil,
	)
	previous, updated, err := repo.Set(context.Background(), requested, "user-1")
	if err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if previous != 8 || updated.QuantityAvailable() != 10 {
		t.Fatalf("Set result previous=%d updated=%+v", previous, updated)
	}
	if !tx.committed {
		t.Fatal("Set did not commit its transaction")
	}

	repo, tx = transactionRepository(
		[]pgx.Row{stockTestRow("pharmacy-1", "product-1", 10, 5)},
		1,
		0,
		nil,
	)
	requested, _ = domain.NewStock("pharmacy-1", "product-1", 3, 0, time.Time{})
	_, _, err = repo.Set(context.Background(), requested, "user-1")
	if !errors.Is(err, domain.ErrOnHandBelowReservedQuantity) {
		t.Fatalf("Set below reserved quantity error = %v", err)
	}
	if !tx.rolledBack {
		t.Fatal("Set validation failure did not roll back")
	}

	repo, tx = transactionRepository(nil, 1, 1, errTestQuery)
	requested, _ = domain.NewStock("pharmacy-1", "product-1", 5, 0, time.Time{})
	if _, _, err := repo.Set(context.Background(), requested, "user-1"); !errors.Is(err, errTestQuery) {
		t.Fatalf("Set database error = %v", err)
	}
	if !tx.rolledBack {
		t.Fatal("Set database failure did not roll back")
	}
}

func TestBulkSetStockValidationAndRows(t *testing.T) {
	repo := &Repository{}
	if _, _, err := repo.BulkSet(context.Background(), " ", nil, "user-1"); !errors.Is(err, domain.ErrInvalidStockKey) {
		t.Fatalf("BulkSet invalid pharmacy error = %v", err)
	}

	changes, rowErrors, err := repo.BulkSet(context.Background(), "pharmacy-1", nil, "user-1")
	if err != nil || len(changes) != 0 || len(rowErrors) != 0 {
		t.Fatalf("BulkSet empty input = (%v, %v, %v)", changes, rowErrors, err)
	}

	repo, tx := transactionRepository(
		[]pgx.Row{
			stockTestRow("pharmacy-1", "product-1", 10, 0),
			stockTestRow("pharmacy-1", "product-1", 12, 0),
		},
		1,
		0,
		nil,
	)
	changes, rowErrors, err = repo.BulkSet(
		context.Background(),
		"pharmacy-1",
		[]ports.StockUpdate{
			{ProductID: "product-1", QuantityOnHand: 12},
			{ProductID: "product-1", QuantityOnHand: 15},
			{ProductID: "product-2", QuantityOnHand: -1},
		},
		"user-1",
	)
	if err != nil {
		t.Fatalf("BulkSet returned error: %v", err)
	}
	if len(changes) != 1 || len(rowErrors) != 2 {
		t.Fatalf("BulkSet returned %d changes and %d row errors", len(changes), len(rowErrors))
	}
	if rowErrors[0].Message != "duplicate product ID in bulk update" {
		t.Fatalf("duplicate error = %q", rowErrors[0].Message)
	}
	if !tx.committed {
		t.Fatal("BulkSet did not commit")
	}

	repo, tx = transactionRepository(nil, 1, 1, errTestQuery)
	changes, rowErrors, err = repo.BulkSet(
		context.Background(),
		"pharmacy-1",
		[]ports.StockUpdate{{ProductID: "product-1", QuantityOnHand: 4}},
		"user-1",
	)
	if !errors.Is(err, errTestQuery) || changes != nil || rowErrors != nil {
		t.Fatalf("BulkSet database error = (%v, %v, %v)", changes, rowErrors, err)
	}
	if !tx.rolledBack {
		t.Fatal("BulkSet database failure did not roll back")
	}
}

func TestIsStockValidationError(t *testing.T) {
	for _, err := range []error{
		domain.ErrInvalidStockKey,
		domain.ErrNegativeStock,
		domain.ErrOnHandBelowReservedQuantity,
	} {
		if !isStockValidationError(err) {
			t.Errorf("isStockValidationError(%v) = false", err)
		}
	}
	if isStockValidationError(errors.New("database failure")) {
		t.Fatal("database failure was classified as a validation error")
	}
}

func TestReserveStockValidationAndSuccess(t *testing.T) {
	ctx := context.Background()
	key := ports.StockKey{PharmacyID: "pharmacy-1", ProductID: "product-1"}
	repo := &Repository{}

	if _, _, err := repo.Reserve(ctx, " ", key, 1); !errors.Is(err, errEmptyHoldID) {
		t.Fatalf("Reserve empty hold error = %v", err)
	}
	if _, _, err := repo.Reserve(ctx, "hold-1", ports.StockKey{}, 1); !errors.Is(err, domain.ErrInvalidStockKey) {
		t.Fatalf("Reserve invalid key error = %v", err)
	}
	if _, _, err := repo.Reserve(ctx, "hold-1", key, 0); !errors.Is(err, domain.ErrInvalidReservationQuantity) {
		t.Fatalf("Reserve invalid quantity error = %v", err)
	}

	repo, tx := transactionRepository(
		[]pgx.Row{
			reservationRow("pharmacy-1", "product-1", 2, reservationUnavailable),
			stockTestRow("pharmacy-1", "product-1", 10, 0),
			stockTestRow("pharmacy-1", "product-1", 10, 2),
		},
		1,
		0,
		nil,
	)
	stock, reserved, err := repo.Reserve(ctx, "hold-1", key, 2)
	if err != nil || !reserved || stock.QuantityReserved != 2 {
		t.Fatalf("Reserve = (%+v, %v, %v)", stock, reserved, err)
	}
	if !tx.committed {
		t.Fatal("Reserve did not commit")
	}
}

func TestReserveIdempotencyAndUnavailableStock(t *testing.T) {
	ctx := context.Background()
	key := ports.StockKey{PharmacyID: "pharmacy-1", ProductID: "product-1"}

	for _, test := range []struct {
		status string
		want   bool
	}{
		{reservationReserved, true},
		{reservationCommitted, true},
		{reservationUnavailable, false},
		{reservationReleased, false},
	} {
		repo, _ := transactionRepository(
			[]pgx.Row{
				reservationRow("pharmacy-1", "product-1", 2, test.status),
				stockTestRow("pharmacy-1", "product-1", 10, 2),
			},
			0,
			0,
			nil,
		)
		_, reserved, err := repo.Reserve(ctx, "hold-1", key, 2)
		if err != nil || reserved != test.want {
			t.Errorf("Reserve status %s = reserved %v, error %v", test.status, reserved, err)
		}
	}

	repo, _ := transactionRepository(
		[]pgx.Row{
			reservationRow("pharmacy-1", "product-1", 2, reservationUnavailable),
			stockTestRow("pharmacy-1", "product-1", 1, 0),
		},
		1,
		0,
		nil,
	)
	stock, reserved, err := repo.Reserve(ctx, "hold-2", key, 2)
	if err != nil || reserved || stock.QuantityAvailable() != 1 {
		t.Fatalf("Reserve insufficient stock = (%+v, %v, %v)", stock, reserved, err)
	}

	repo, _ = transactionRepository(
		[]pgx.Row{
			reservationRow("pharmacy-1", "product-1", 2, reservationUnavailable),
			fakeRow{err: pgx.ErrNoRows},
		},
		1,
		0,
		nil,
	)
	stock, reserved, err = repo.Reserve(ctx, "hold-3", key, 2)
	if err != nil || reserved || stock.PharmacyID != key.PharmacyID {
		t.Fatalf("Reserve with missing stock = (%+v, %v, %v)", stock, reserved, err)
	}
}

func TestReserveRejectsMismatchedAndUnknownReservations(t *testing.T) {
	ctx := context.Background()
	key := ports.StockKey{PharmacyID: "pharmacy-1", ProductID: "product-1"}

	repo, _ := transactionRepository(
		[]pgx.Row{reservationRow("another-pharmacy", "product-1", 2, reservationReserved)},
		0,
		0,
		nil,
	)
	if _, _, err := repo.Reserve(ctx, "hold-1", key, 2); !errors.Is(err, errReservationKeyMismatch) {
		t.Fatalf("Reserve mismatch error = %v", err)
	}

	repo, _ = transactionRepository(
		[]pgx.Row{
			reservationRow("pharmacy-1", "product-1", 2, "UNKNOWN"),
			stockTestRow("pharmacy-1", "product-1", 10, 2),
		},
		0,
		0,
		nil,
	)
	if _, _, err := repo.Reserve(ctx, "hold-2", key, 2); !errors.Is(err, errReservationConflict) {
		t.Fatalf("Reserve unknown status error = %v", err)
	}
}

func TestCommitReservationSuccessIdempotencyAndErrors(t *testing.T) {
	ctx := context.Background()

	repo := &Repository{}
	if _, err := repo.CommitReservation(ctx, " "); !errors.Is(err, errEmptyHoldID) {
		t.Fatalf("CommitReservation empty hold error = %v", err)
	}

	repo, tx := transactionRepository(
		[]pgx.Row{
			reservationRow("pharmacy-1", "product-1", 2, reservationReserved),
			stockTestRow("pharmacy-1", "product-1", 10, 2),
			stockTestRow("pharmacy-1", "product-1", 8, 0),
		},
		1,
		0,
		nil,
	)
	stock, err := repo.CommitReservation(ctx, "hold-1")
	if err != nil || stock.QuantityOnHand != 8 || stock.QuantityReserved != 0 {
		t.Fatalf("CommitReservation = (%+v, %v)", stock, err)
	}
	if !tx.committed {
		t.Fatal("CommitReservation did not commit")
	}

	repo, _ = transactionRepository(
		[]pgx.Row{
			reservationRow("pharmacy-1", "product-1", 2, reservationCommitted),
			stockTestRow("pharmacy-1", "product-1", 8, 0),
		},
		1,
		0,
		nil,
	)
	if _, err := repo.CommitReservation(ctx, "hold-1"); err != nil {
		t.Fatalf("idempotent CommitReservation: %v", err)
	}

	repo, _ = transactionRepository(
		[]pgx.Row{
			reservationRow("pharmacy-1", "product-1", 2, reservationUnavailable),
			stockTestRow("pharmacy-1", "product-1", 8, 0),
		},
		1,
		0,
		nil,
	)
	if _, err := repo.CommitReservation(ctx, "hold-2"); !errors.Is(err, errReservationConflict) {
		t.Fatalf("CommitReservation conflict error = %v", err)
	}

	repo, _ = transactionRepository(
		[]pgx.Row{fakeRow{err: pgx.ErrNoRows}},
		1,
		0,
		nil,
	)
	if _, err := repo.CommitReservation(ctx, "missing"); !errors.Is(err, errReservationNotFound) {
		t.Fatalf("CommitReservation missing hold error = %v", err)
	}
}

func TestReleaseReservationSuccessIdempotencyAndErrors(t *testing.T) {
	ctx := context.Background()

	repo := &Repository{}
	if _, err := repo.ReleaseReservation(ctx, " "); !errors.Is(err, errEmptyHoldID) {
		t.Fatalf("ReleaseReservation empty hold error = %v", err)
	}

	repo, tx := transactionRepository(
		[]pgx.Row{
			reservationRow("pharmacy-1", "product-1", 2, reservationReserved),
			stockTestRow("pharmacy-1", "product-1", 10, 2),
			stockTestRow("pharmacy-1", "product-1", 10, 0),
		},
		1,
		0,
		nil,
	)
	stock, err := repo.ReleaseReservation(ctx, "hold-1")
	if err != nil || stock.QuantityAvailable() != 10 || stock.QuantityReserved != 0 {
		t.Fatalf("ReleaseReservation = (%+v, %v)", stock, err)
	}
	if !tx.committed {
		t.Fatal("ReleaseReservation did not commit")
	}

	for _, state := range []string{reservationReleased, reservationUnavailable} {
		repo, _ = transactionRepository(
			[]pgx.Row{
				reservationRow("pharmacy-1", "product-1", 2, state),
				stockTestRow("pharmacy-1", "product-1", 10, 0),
			},
			1,
			0,
			nil,
		)
		if _, err := repo.ReleaseReservation(ctx, "hold-1"); err != nil {
			t.Errorf("idempotent ReleaseReservation state %s: %v", state, err)
		}
	}

	repo, _ = transactionRepository(
		[]pgx.Row{
			reservationRow("pharmacy-1", "product-1", 2, reservationCommitted),
			stockTestRow("pharmacy-1", "product-1", 8, 0),
		},
		1,
		0,
		nil,
	)
	if _, err := repo.ReleaseReservation(ctx, "hold-2"); !errors.Is(err, errReservationConflict) {
		t.Fatalf("ReleaseReservation conflict error = %v", err)
	}

	repo, _ = transactionRepository(
		[]pgx.Row{fakeRow{err: pgx.ErrNoRows}},
		1,
		0,
		nil,
	)
	if _, err := repo.ReleaseReservation(ctx, "missing"); !errors.Is(err, errReservationNotFound) {
		t.Fatalf("ReleaseReservation missing hold error = %v", err)
	}
}
