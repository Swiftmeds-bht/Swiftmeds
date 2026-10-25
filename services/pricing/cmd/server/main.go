// Command server runs the Pricing service.
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
	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/adapters/postgres"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/migrations"
)

const (
	serviceName = "pricing"
	defaultPort = 50063
	portEnv     = "PRICING_GRPC_PORT"
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
	databaseURL := cfg.Required("PRICING_DATABASE_URL")
	logLevel := cfg.String("LOG_LEVEL", "info")
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
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

	// Bring pricing_db up to date before serving any request.
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
	svc := app.NewService(repo, time.Now)

	srv := grpcx.NewServer(log)
	pricingv1.RegisterPricingServiceServer(srv, grpcadapter.NewHandler(svc))

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
