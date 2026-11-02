package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/migrations"
)

var t0 = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

// newTestRepo connects to the database in FRESHNESS_TEST_DATABASE_URL and works in
// a new schema that is dropped afterwards, so it never touches real data.
// Without the variable the test is skipped, like the RabbitMQ integration test.
func newTestRepo(t *testing.T) (*Repository, *sql.DB) {
	t.Helper()
	baseURL := os.Getenv("FRESHNESS_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("set FRESHNESS_TEST_DATABASE_URL to run the Postgres integration tests")
	}
	ctx := context.Background()

	admin, err := sql.Open("pgx", baseURL)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	schema := fmt.Sprintf("freshness_test_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close()
	})

	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatalf("open test connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := Migrate(ctx, db, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepository(db), db
}

func rating(pharmacyID, productID string, confirmedAt time.Time, c domain.Confidence) domain.Rating {
	return domain.Rating{
		Confirmation: domain.Confirmation{Key: domain.Key{PharmacyID: pharmacyID, ProductID: productID}, ConfirmedAt: confirmedAt},
		Confidence:   c,
	}
}

func TestMigrateIsSafeToRunTwice(t *testing.T) {
	_, db := newTestRepo(t)

	if err := Migrate(context.Background(), db, migrations.FS); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestMigrateStopsAtABrokenFileAndNamesIt(t *testing.T) {
	_, db := newTestRepo(t)
	fsys := fstest.MapFS{
		"002_broken.up.sql":    {Data: []byte("THIS IS NOT SQL;")},
		"003_never_run.up.sql": {Data: []byte("CREATE TABLE never_run (id INT);")},
	}

	err := Migrate(context.Background(), db, fsys)

	if err == nil || !strings.Contains(err.Error(), "002_broken.up.sql") {
		t.Fatalf("err = %v, want an error naming 002_broken.up.sql", err)
	}
	var exists bool
	if err := db.QueryRow("SELECT to_regclass('never_run') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("003_never_run.up.sql ran after the broken file")
	}
}

func TestSaveAndGetManyFollowRequestOrderAndSkipMissingKeys(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	if _, err := repo.Save(ctx, rating("ph-1", "med-1", t0, domain.High)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Save(ctx, rating("ph-2", "med-1", t0.Add(-7*time.Hour), domain.Medium)); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetMany(ctx, []domain.Key{
		{PharmacyID: "ph-2", ProductID: "med-1"},
		{PharmacyID: "ph-9", ProductID: "med-1"}, // never saved
		{PharmacyID: "ph-1", ProductID: "med-1"},
	})

	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].PharmacyID != "ph-2" || got[1].PharmacyID != "ph-1" {
		t.Fatalf("got %+v, want ph-2 then ph-1", got)
	}
	if got[0].Confidence != domain.Medium || !got[0].ConfirmedAt.Equal(t0.Add(-7*time.Hour)) {
		t.Errorf("ph-2 row = %+v, want MEDIUM confirmed 7h before t0", got[0])
	}
	if got[1].ConfirmedAt.Location() != time.UTC {
		t.Errorf("confirmed_at location = %v, want UTC", got[1].ConfirmedAt.Location())
	}
}

func TestGetManyWithNoKeys(t *testing.T) {
	repo, _ := newTestRepo(t)

	got, err := repo.GetMany(context.Background(), nil)

	if err != nil || len(got) != 0 {
		t.Fatalf("got (%v, %v), want (empty, nil)", got, err)
	}
}

func TestSaveOnlyReplacesWithANewerConfirmation(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	if _, err := repo.Save(ctx, rating("ph-1", "med-1", t0, domain.High)); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name        string
		confirmedAt time.Time
		confidence  domain.Confidence
		wantTime    time.Time
		wantConf    domain.Confidence
	}{
		{"older is ignored", t0.Add(-time.Hour), domain.Medium, t0, domain.High},
		{"equal is ignored", t0, domain.Low, t0, domain.High},
		{"newer replaces", t0.Add(time.Hour), domain.Low, t0.Add(time.Hour), domain.Low},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := repo.Save(ctx, rating("ph-1", "med-1", c.confirmedAt, c.confidence))
			if err != nil {
				t.Fatal(err)
			}
			if !got.ConfirmedAt.Equal(c.wantTime) || got.Confidence != c.wantConf {
				t.Errorf("Save returned %+v, want confirmed_at %v and %s", got, c.wantTime, c.wantConf)
			}
		})
	}
}

func TestAllAndUpdateConfidence(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	for _, id := range []string{"ph-1", "ph-2", "ph-3"} {
		if _, err := repo.Save(ctx, rating(id, "med-1", t0, domain.High)); err != nil {
			t.Fatal(err)
		}
	}

	err := repo.UpdateConfidence(ctx, []domain.Rating{
		rating("ph-1", "med-1", t0, domain.Medium),
		rating("ph-3", "med-1", t0, domain.Low),
	})
	if err != nil {
		t.Fatal(err)
	}

	all, err := repo.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]domain.Confidence{}
	for _, r := range all {
		got[r.PharmacyID] = r.Confidence
	}
	want := map[string]domain.Confidence{"ph-1": domain.Medium, "ph-2": domain.High, "ph-3": domain.Low}
	for id, c := range want {
		if got[id] != c {
			t.Errorf("%s confidence = %s, want %s", id, got[id], c)
		}
	}
}

func TestUpdateConfidenceWithNothingToUpdate(t *testing.T) {
	repo, _ := newTestRepo(t)

	if err := repo.UpdateConfidence(context.Background(), nil); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestDatabaseRejectsAnUnknownConfidence(t *testing.T) {
	repo, _ := newTestRepo(t)

	_, err := repo.Save(context.Background(), rating("ph-1", "med-1", t0, domain.Confidence("MAYBE")))

	if err == nil {
		t.Fatal("err = nil, want the CHECK constraint to reject MAYBE")
	}
}

func TestQueriesReturnErrorsWhenTheDatabaseIsClosed(t *testing.T) {
	repo, db := newTestRepo(t)
	_ = db.Close()
	ctx := context.Background()
	keys := []domain.Key{{PharmacyID: "ph-1", ProductID: "med-1"}}

	if _, err := repo.Save(ctx, rating("ph-1", "med-1", t0, domain.High)); err == nil {
		t.Error("Save: err = nil, want an error")
	}
	if _, err := repo.GetMany(ctx, keys); err == nil {
		t.Error("GetMany: err = nil, want an error")
	}
	if _, err := repo.All(ctx); err == nil {
		t.Error("All: err = nil, want an error")
	}
	if err := repo.UpdateConfidence(ctx, []domain.Rating{rating("ph-1", "med-1", t0, domain.Low)}); err == nil {
		t.Error("UpdateConfidence: err = nil, want an error")
	}
}

// The real repository behind the real use cases, including Decay.
func TestServiceOnPostgresRecordsAndDecays(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	now := t0
	svc := app.NewService(repo, domain.DefaultThresholds(), func() time.Time { return now })

	if _, err := svc.RecordConfirmation(ctx, "ph-1", "med-1", t0); err != nil {
		t.Fatal(err)
	}
	first, err := svc.Decay(ctx, t0.Add(6*time.Hour))
	if err != nil || first != 1 {
		t.Fatalf("first Decay = (%d, %v), want (1, nil)", first, err)
	}
	second, err := svc.Decay(ctx, t0.Add(6*time.Hour))
	if err != nil || second != 0 {
		t.Fatalf("second Decay = (%d, %v), want (0, nil)", second, err)
	}

	got, err := svc.GetConfidence(ctx, "ph-1", "med-1")
	if err != nil || got.Confidence != domain.Medium {
		t.Fatalf("got (%+v, %v), want MEDIUM", got, err)
	}
}
