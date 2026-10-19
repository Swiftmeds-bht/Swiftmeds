package messaging

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

type fakeExecer struct {
	queries []string
	err     error
}

func (f *fakeExecer) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	f.queries = append(f.queries, query)
	return nil, f.err
}

func TestEnsureSchemaCreatesDedupTable(t *testing.T) {
	db := &fakeExecer{}

	if err := EnsureSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if len(db.queries) != 1 {
		t.Fatalf("ran %d queries, want 1", len(db.queries))
	}
	q := db.queries[0]
	if !strings.Contains(q, "CREATE TABLE IF NOT EXISTS swiftmed_processed_events") {
		t.Errorf("query is not an idempotent create of the dedup table: %s", q)
	}
}

func TestEnsureSchemaWrapsDatabaseError(t *testing.T) {
	dbDown := errors.New("connection refused")

	err := EnsureSchema(context.Background(), &fakeExecer{err: dbDown})
	if !errors.Is(err, dbDown) {
		t.Fatalf("err = %v, want wrapped database error", err)
	}
}

func TestEnsureSchemaRejectsNilDatabase(t *testing.T) {
	if err := EnsureSchema(context.Background(), nil); err == nil {
		t.Fatal("EnsureSchema(nil) returned nil error")
	}
}
