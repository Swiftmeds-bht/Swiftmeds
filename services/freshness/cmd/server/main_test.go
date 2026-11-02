package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	freshnessv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/freshness/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
)

const dbURL = "postgres://u:p@localhost:5432/freshness_db"

func TestLoadSettingsDefaults(t *testing.T) {
	s, err := loadSettings(config.NewFromMap(map[string]string{"FRESHNESS_DATABASE_URL": dbURL}))

	if err != nil {
		t.Fatal(err)
	}
	if s.port != 50069 || s.databaseURL != dbURL || s.logLevel != "info" {
		t.Errorf("got %+v, want port 50069, the database URL and log level info", s)
	}
	if s.thresholds != domain.DefaultThresholds() {
		t.Errorf("thresholds = %+v, want the defaults", s.thresholds)
	}
}

func TestLoadSettingsReadsConfiguredValues(t *testing.T) {
	s, err := loadSettings(config.NewFromMap(map[string]string{
		"FRESHNESS_DATABASE_URL":  dbURL,
		"FRESHNESS_GRPC_PORT":     "6000",
		"LOG_LEVEL":               "debug",
		"FRESHNESS_HIGH_WITHIN":   "1h",
		"FRESHNESS_MEDIUM_WITHIN": "90m",
	}))

	if err != nil {
		t.Fatal(err)
	}
	if s.port != 6000 || s.logLevel != "debug" {
		t.Errorf("got port %d and log level %q, want 6000 and debug", s.port, s.logLevel)
	}
	if s.thresholds.HighWithin != time.Hour || s.thresholds.MediumWithin != 90*time.Minute {
		t.Errorf("thresholds = %+v, want 1h and 90m", s.thresholds)
	}
}

func TestLoadSettingsReportsEveryProblem(t *testing.T) {
	_, err := loadSettings(config.NewFromMap(map[string]string{
		"FRESHNESS_HIGH_WITHIN": "soon", // not a duration, and no database URL either
	}))

	if err == nil {
		t.Fatal("err = nil, want an error")
	}
	for _, want := range []string{"FRESHNESS_DATABASE_URL", "FRESHNESS_HIGH_WITHIN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadSettingsRejectsBadThresholds(t *testing.T) {
	cases := map[string]map[string]string{
		"medium shorter than high": {"FRESHNESS_HIGH_WITHIN": "2h", "FRESHNESS_MEDIUM_WITHIN": "1h"},
		"medium equal to high":     {"FRESHNESS_HIGH_WITHIN": "1h", "FRESHNESS_MEDIUM_WITHIN": "1h"},
		"zero high":                {"FRESHNESS_HIGH_WITHIN": "0s"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			env["FRESHNESS_DATABASE_URL"] = dbURL

			_, err := loadSettings(config.NewFromMap(env))

			if err == nil || !strings.Contains(err.Error(), "FRESHNESS_HIGH_WITHIN") {
				t.Fatalf("err = %v, want a threshold error", err)
			}
		})
	}
}

// startServer runs serve on a free port and returns the port and a function that
// stops the server and returns what serve returned.
func startServer(t *testing.T, s settings) (port int, stop func() error) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, s, lis) }()
	t.Cleanup(cancel)
	return lis.Addr().(*net.TCPAddr).Port, func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			return fmt.Errorf("server did not stop within 5s")
		}
	}
}

func TestHealthcheck(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	srv := grpcx.NewServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.ServeListener(ctx, lis) }()

	if code := healthcheck(port); code != 0 {
		t.Errorf("healthcheck of a running server = %d, want 0", code)
	}

	cancel()
	<-done
	if code := healthcheck(port); code != 1 {
		t.Errorf("healthcheck of a stopped server = %d, want 1", code)
	}
}

func TestServeFailsClearlyWhenTheDatabaseIsUnreachable(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lis.Close() }()
	s := settings{
		databaseURL: "postgres://u:p@127.0.0.1:1/freshness_db?sslmode=disable", // nothing listens on port 1
		thresholds:  domain.DefaultThresholds(),
		logLevel:    "error",
	}

	err = serve(context.Background(), s, lis)

	if err == nil || !strings.Contains(err.Error(), "connect to database") {
		t.Fatalf("err = %v, want a \"connect to database\" error", err)
	}
}

// testDatabaseURL returns a URL that works in a new schema (dropped afterwards),
// so the test never touches real data. Without FRESHNESS_TEST_DATABASE_URL the test is skipped.
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	baseURL := os.Getenv("FRESHNESS_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("set FRESHNESS_TEST_DATABASE_URL to run the Postgres integration tests")
	}
	admin, err := sql.Open("pgx", baseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("freshness_main_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		_ = admin.Close()
	})

	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func TestServeMigratesAnswersCallsAndStopsCleanly(t *testing.T) {
	port, stop := startServer(t, settings{
		databaseURL: testDatabaseURL(t),
		thresholds:  domain.DefaultThresholds(),
		logLevel:    "error",
	})

	conn, err := grpc.NewClient(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	client := freshnessv1.NewFreshnessServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := &commonv1.StockKey{PharmacyId: "ph-1", ProductId: "med-1"}
	rec, err := client.RecordConfirmation(ctx, &freshnessv1.RecordConfirmationRequest{
		Key: key, ConfirmedAt: timestamppb.New(time.Now().Add(-7 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("RecordConfirmation: %v", err)
	}
	if got := rec.GetFreshness().GetScore(); got != 0.6 {
		t.Errorf("score of a 7 hour old confirmation = %v, want 0.6 (MEDIUM)", got)
	}

	got, err := client.GetFreshness(ctx, &freshnessv1.GetFreshnessRequest{Keys: []*commonv1.StockKey{key}})
	if err != nil || len(got.GetScores()) != 1 {
		t.Fatalf("GetFreshness = (%v, %v), want one score", got, err)
	}
	if code := healthcheck(port); code != 0 {
		t.Errorf("healthcheck = %d, want 0", code)
	}

	if err := stop(); err != nil {
		t.Errorf("serve returned %v after the context was cancelled, want nil", err)
	}
}
