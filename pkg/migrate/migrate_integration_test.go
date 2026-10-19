//go:build integration

package migrate

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"sync"
	"testing"
	"testing/fstest"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Run with a throwaway database:
//
//	POSTGRES_URL=postgres://swiftmed:change-me@localhost:5432/migrate_test?sslmode=disable \
//	  go test -tags integration ./migrate/
func TestUpAgainstPostgres(t *testing.T) {
	url := os.Getenv("POSTGRES_URL")
	if url == "" {
		t.Skip("set POSTGRES_URL to run the migrate integration test")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	reset := func() {
		for _, q := range []string{`DROP TABLE IF EXISTS mt_items`, `DROP TABLE IF EXISTS schema_migrations`} {
			if _, err := db.ExecContext(ctx, q); err != nil {
				t.Fatal(err)
			}
		}
	}
	reset()
	t.Cleanup(reset)

	fsys := fstest.MapFS{
		"001_create.up.sql": {Data: []byte(`CREATE TABLE mt_items (id INT PRIMARY KEY)`)},
		"002_seed.up.sql":   {Data: []byte(`INSERT INTO mt_items VALUES (1)`)},
	}

	t.Run("applies pending migrations in order", func(t *testing.T) {
		got, err := Up(ctx, db, fsys)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"001_create", "002_seed"}; !reflect.DeepEqual(got, want) {
			t.Errorf("Up() = %v, want %v", got, want)
		}
	})

	t.Run("second run applies nothing", func(t *testing.T) {
		got, err := Up(ctx, db, fsys)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("Up() second run = %v, want none", got)
		}
	})

	t.Run("failing migration is rolled back and not recorded", func(t *testing.T) {
		bad := fstest.MapFS{
			"001_create.up.sql": fsys["001_create.up.sql"],
			"002_seed.up.sql":   fsys["002_seed.up.sql"],
			"003_bad.up.sql":    {Data: []byte(`INSERT INTO mt_items VALUES (2); SELECT * FROM no_such_table`)},
		}
		if _, err := Up(ctx, db, bad); err == nil {
			t.Fatal("Up() with a broken migration returned nil error")
		}
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM mt_items`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("mt_items has %d rows, want 1 (003 must roll back)", n)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version = '003_bad'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Error("003_bad was recorded in schema_migrations despite failing")
		}
	})

	t.Run("concurrent callers apply each migration once", func(t *testing.T) {
		reset()
		var wg sync.WaitGroup
		results := make([][]string, 4)
		errs := make([]error, 4)
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				results[i], errs[i] = Up(ctx, db, fsys)
			}(i)
		}
		wg.Wait()

		total := 0
		for i := range results {
			if errs[i] != nil {
				t.Fatalf("caller %d: %v", i, errs[i])
			}
			total += len(results[i])
		}
		if total != 2 {
			t.Errorf("migrations applied %d times in total, want 2", total)
		}
	})
}
