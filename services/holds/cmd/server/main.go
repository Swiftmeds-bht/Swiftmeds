// Command server runs the Holds service.
//
// main only wires things together: config -> logger -> adapters -> app -> gRPC server.
// Run "server healthcheck" to check a running instance (used by the Docker HEALTHCHECK).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/postgres"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/migrations"
)

const (
	serviceName = "holds"
	defaultPort = 50065
	portEnv     = "HOLDS_GRPC_PORT"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.New()
	port := cfg.Int(portEnv, defaultPort)
	databaseURL := cfg.Required("HOLDS_DATABASE_URL")
	maxActive := cfg.Int("HOLDS_MAX_ACTIVE_PER_USER", app.DefaultMaxActivePerUser)
	holdDuration := cfg.Duration("HOLDS_HOLD_DURATION", app.DefaultHoldDuration)
	logLevel := cfg.String("LOG_LEVEL", "info")
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// Zero would switch the limit off, and a hold that lasts no time cannot be collected.
	if maxActive < 1 {
		return fmt.Errorf("config: HOLDS_MAX_ACTIVE_PER_USER must be at least 1, got %d", maxActive)
	}
	if holdDuration <= 0 {
		return fmt.Errorf("config: HOLDS_HOLD_DURATION must be greater than zero, got %s", holdDuration)
	}

	log := logger.New(serviceName, logLevel)

	db, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer db.Close()
	startCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.Ping(startCtx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}

	// Bring holds_db up to date before serving any request.
	sqlDB := stdlib.OpenDBFromPool(db)
	defer func() { _ = sqlDB.Close() }()
	applied, err := migrate.Up(startCtx, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Info("database migrations up to date", slog.Int("applied", len(applied)), slog.Any("versions", applied))

	// Cancelled on Ctrl+C or when Docker stops the container (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Outbound adapters -> use cases -> inbound adapter.
	repo := postgres.NewRepository(db)
	svc := app.NewService(repo, app.Config{MaxActivePerUser: maxActive, HoldDuration: holdDuration}, time.Now)
	log.Info("holds settings",
		slog.Int("max_active_per_user", maxActive),
		slog.Duration("hold_duration", holdDuration))

	srv := grpcx.NewServer(log)
	holdsv1.RegisterHoldsServiceServer(srv, grpcadapter.NewHandler(svc))

	return srv.Serve(ctx, fmt.Sprintf(":%d", port))
}

func healthcheck() int {
	port := config.New().Int(portEnv, defaultPort)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := grpcx.CheckHealth(ctx, fmt.Sprintf("localhost:%d", port)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
