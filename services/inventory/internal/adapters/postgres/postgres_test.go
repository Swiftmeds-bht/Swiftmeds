package postgres

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var errTestQuery = errors.New("test query failure")

type fakeDB struct {
	beginFn    func(context.Context) (pgx.Tx, error)
	queryFn    func(context.Context, string, ...any) (pgx.Rows, error)
	queryRowFn func(context.Context, string, ...any) pgx.Row
}

func (db *fakeDB) Begin(ctx context.Context) (pgx.Tx, error) {
	if db.beginFn != nil {
		return db.beginFn(ctx)
	}
	return &fakeTx{}, nil
}

func (db *fakeDB) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	if db.queryFn != nil {
		return db.queryFn(ctx, query, args...)
	}
	return &fakeRows{}, nil
}

func (db *fakeDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	if db.queryRowFn != nil {
		return db.queryRowFn(ctx, query, args...)
	}
	return fakeRow{err: pgx.ErrNoRows}
}

type fakeTx struct {
	pgx.Tx
	execFn     func(context.Context, string, ...any) (pgconn.CommandTag, error)
	queryRowFn func(context.Context, string, ...any) pgx.Row
	committed  bool
	rolledBack bool
}

func (tx *fakeTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if tx.execFn != nil {
		return tx.execFn(ctx, query, args...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (tx *fakeTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	if tx.queryRowFn != nil {
		return tx.queryRowFn(ctx, query, args...)
	}
	return fakeRow{err: pgx.ErrNoRows}
}

func (tx *fakeTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *fakeTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

type fakeRow struct {
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != len(row.values) {
		return errors.New("wrong number of scan destinations")
	}

	for i, target := range dest {
		dst := reflect.ValueOf(target)
		if dst.Kind() != reflect.Pointer || dst.IsNil() {
			return errors.New("scan destination must be a non-nil pointer")
		}

		if row.values[i] == nil {
			dst.Elem().Set(reflect.Zero(dst.Elem().Type()))
			continue
		}

		src := reflect.ValueOf(row.values[i])
		if !src.Type().AssignableTo(dst.Elem().Type()) {
			return errors.New("scan value has the wrong type")
		}
		dst.Elem().Set(src)
	}

	return nil
}

type fakeRows struct {
	pgx.Rows
	values       [][]any
	current      []any
	next         int
	scanErr      error
	iterationErr error
	closed       bool
}

func (rows *fakeRows) Next() bool {
	if rows.next >= len(rows.values) {
		return false
	}
	rows.current = rows.values[rows.next]
	rows.next++
	return true
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.scanErr != nil {
		return rows.scanErr
	}
	return fakeRow{values: rows.current}.Scan(dest...)
}

func (rows *fakeRows) Err() error {
	return rows.iterationErr
}

func (rows *fakeRows) Close() {
	rows.closed = true
}

func stockValues(pharmacyID, productID string, onHand, reserved int32) []any {
	return []any{
		pharmacyID,
		productID,
		onHand,
		reserved,
		time.Date(2026, time.October, 29, 12, 0, 0, 0, time.UTC),
	}
}

func TestScanStock(t *testing.T) {
	stock, err := scanStock(fakeRow{values: stockValues("pharmacy-1", "product-1", 8, 2)})
	if err != nil {
		t.Fatalf("scanStock returned error: %v", err)
	}
	if stock.PharmacyID != "pharmacy-1" || stock.ProductID != "product-1" ||
		stock.QuantityAvailable() != 6 {
		t.Fatalf("scanStock returned unexpected stock: %+v", stock)
	}

	if _, err := scanStock(fakeRow{err: errTestQuery}); !errors.Is(err, errTestQuery) {
		t.Fatalf("scanStock error = %v, want scan error", err)
	}

	if _, err := scanStock(fakeRow{values: stockValues("pharmacy-1", "product-1", -1, 0)}); err == nil {
		t.Fatal("scanStock accepted negative stock")
	}
}

func TestRepositoryGet(t *testing.T) {
	repo := &Repository{db: &fakeDB{}}

	if _, found, err := repo.Get(context.Background(), ports.StockKey{ProductID: "product-1"}); !errors.Is(err, domain.ErrInvalidStockKey) || found {
		t.Fatalf("Get invalid key = found %v, error %v", found, err)
	}

	repo.db = &fakeDB{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return fakeRow{err: pgx.ErrNoRows}
		},
	}
	if _, found, err := repo.Get(context.Background(), ports.StockKey{PharmacyID: "pharmacy-1", ProductID: "missing"}); err != nil || found {
		t.Fatalf("Get missing row = found %v, error %v", found, err)
	}

	repo.db = &fakeDB{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return fakeRow{err: errTestQuery}
		},
	}
	if _, _, err := repo.Get(context.Background(), ports.StockKey{PharmacyID: "pharmacy-1", ProductID: "product-1"}); !errors.Is(err, errTestQuery) {
		t.Fatalf("Get error = %v, want wrapped query error", err)
	}

	repo.db = &fakeDB{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return fakeRow{values: stockValues("pharmacy-1", "product-1", 8, 2)}
		},
	}
	stock, found, err := repo.Get(context.Background(), ports.StockKey{PharmacyID: "pharmacy-1", ProductID: "product-1"})
	if err != nil || !found || stock.QuantityAvailable() != 6 {
		t.Fatalf("Get = (%+v, %v, %v)", stock, found, err)
	}
}

func TestRepositoryGetMany(t *testing.T) {
	repo := &Repository{db: &fakeDB{}}

	stocks, err := repo.GetMany(context.Background(), nil, []string{"product-1"}, false)
	if err != nil || len(stocks) != 0 {
		t.Fatalf("GetMany with no pharmacies = (%v, %v)", stocks, err)
	}

	rowsErr := &fakeRows{values: [][]any{stockValues("pharmacy-1", "product-1", 8, 2)}}
	repo.db = &fakeDB{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return rowsErr, nil
		},
	}
	stocks, err = repo.GetMany(context.Background(), []string{"pharmacy-1"}, []string{"product-1"}, true)
	if err != nil || len(stocks) != 1 || stocks[0].QuantityAvailable() != 6 {
		t.Fatalf("GetMany = (%v, %v)", stocks, err)
	}
	if !rowsErr.closed {
		t.Fatal("GetMany did not close rows")
	}

	repo.db = &fakeDB{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return nil, errTestQuery
		},
	}
	if _, err := repo.GetMany(context.Background(), []string{"pharmacy-1"}, []string{"product-1"}, false); !errors.Is(err, errTestQuery) {
		t.Fatalf("GetMany query error = %v", err)
	}

	repo.db = &fakeDB{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &fakeRows{scanErr: errTestQuery, values: [][]any{stockValues("pharmacy-1", "product-1", 8, 2)}}, nil
		},
	}
	if _, err := repo.GetMany(context.Background(), []string{"pharmacy-1"}, []string{"product-1"}, false); !errors.Is(err, errTestQuery) {
		t.Fatalf("GetMany scan error = %v", err)
	}

	repo.db = &fakeDB{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return &fakeRows{iterationErr: errTestQuery}, nil
		},
	}
	if _, err := repo.GetMany(context.Background(), []string{"pharmacy-1"}, []string{"product-1"}, false); !errors.Is(err, errTestQuery) {
		t.Fatalf("GetMany iteration error = %v", err)
	}
}

func TestRepositoryListByPharmacyPaginationAndValidation(t *testing.T) {
	repo := &Repository{db: &fakeDB{}}

	if _, _, err := repo.ListByPharmacy(context.Background(), " ", 10, ""); !errors.Is(err, domain.ErrInvalidStockKey) {
		t.Fatalf("ListByPharmacy invalid key error = %v", err)
	}
	if _, _, err := repo.ListByPharmacy(context.Background(), "pharmacy-1", 10, "%%%"); err == nil {
		t.Fatal("ListByPharmacy accepted invalid page token")
	}

	rows := &fakeRows{values: [][]any{
		stockValues("pharmacy-1", "product-1", 3, 0),
		stockValues("pharmacy-1", "product-2", 4, 1),
	}}
	repo.db = &fakeDB{
		queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
			if args[0] != "pharmacy-1" || args[1] != "after" || args[2] != int32(2) {
				t.Fatalf("ListByPharmacy query arguments = %#v", args)
			}
			return rows, nil
		},
	}

	stocks, next, err := repo.ListByPharmacy(
		context.Background(),
		"pharmacy-1",
		1,
		encodePageToken("after"),
	)
	if err != nil || len(stocks) != 1 || next != encodePageToken("product-1") {
		t.Fatalf("ListByPharmacy = (%v, %q, %v)", stocks, next, err)
	}

	repo.db = &fakeDB{
		queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
			return nil, errTestQuery
		},
	}
	if _, _, err := repo.ListByPharmacy(context.Background(), "pharmacy-1", 10, ""); !errors.Is(err, errTestQuery) {
		t.Fatalf("ListByPharmacy query error = %v", err)
	}
}

func TestPageTokenHelpersAndPageSizeDefaults(t *testing.T) {
	if got := encodePageToken("product-1"); got == "" {
		t.Fatal("encodePageToken returned an empty token")
	}
	if got, err := decodePageToken(""); err != nil || got != "" {
		t.Fatalf("decode empty token = %q, %v", got, err)
	}
	if got, err := decodePageToken(encodePageToken("product-1")); err != nil || got != "product-1" {
		t.Fatalf("decode token = %q, %v", got, err)
	}

	for _, test := range []struct {
		pageSize  int32
		wantLimit int32
	}{
		{pageSize: 0, wantLimit: 51},
		{pageSize: 200, wantLimit: 101},
	} {
		repo := &Repository{
			db: &fakeDB{
				queryFn: func(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
					if args[2] != test.wantLimit {
						t.Errorf("query limit = %v, want %d", args[2], test.wantLimit)
					}
					return &fakeRows{}, nil
				},
			},
		}
		if _, _, err := repo.ListByPharmacy(context.Background(), "pharmacy-1", test.pageSize, ""); err != nil {
			t.Fatalf("ListByPharmacy page size %d: %v", test.pageSize, err)
		}
	}
}

func TestNewRepositoryRejectsNilPool(t *testing.T) {
	if _, err := NewRepository(nil); err == nil {
		t.Fatal("NewRepository(nil) returned no error")
	}
}
