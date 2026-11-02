// Command server runs the Stock Sync service.
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/resilience"
	stocksyncv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/stock-sync/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/adapters/clients"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/adapters/postgres"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/stock-sync/migrations"
)

const (
	serviceName = "stock-sync"
	defaultPort = 50057
	portEnv     = "STOCK_SYNC_GRPC_PORT"
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
	databaseURL := cfg.Required("STOCK_SYNC_DATABASE_URL")
	upstream := cfg.String("STOCK_SYNC_UPSTREAM", "traefik:50051")
	callTimeout := cfg.Duration("STOCK_SYNC_CALL_TIMEOUT", 800*time.Millisecond)
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

	sqlDB := stdlib.OpenDBFromPool(db)
	defer func() {
		_ = sqlDB.Close()
	}()

	applied, err := migrate.Up(startCtx, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Info(
		"database migrations up to date",
		slog.Int("applied", len(applied)),
		slog.Any("versions", applied),
	)

	dial := func(authority string) (*grpc.ClientConn, error) {
		return grpc.NewClient(
			upstream,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithAuthority(authority),
			grpc.WithChainUnaryInterceptor(
				resilience.UnaryClientInterceptor(resilience.Config{
					Timeout: callTimeout,
					Retry: resilience.RetryConfig{
						MaxAttempts: 2,
						BaseDelay:   50 * time.Millisecond,
					},
				}),
			),
		)
	}

	connections := make(map[string]*grpc.ClientConn, 3)
	for _, authority := range []string{"catalog", "inventory", "pricing"} {
		connection, err := dial(authority)
		if err != nil {
			return fmt.Errorf("create %s client: %w", authority, err)
		}
		connections[authority] = connection
		defer func() {
			_ = connection.Close()
		}()
	}

	service := app.NewService(
		clients.NewCatalogClient(connections["catalog"]),
		clients.NewInventoryClient(connections["inventory"]),
		clients.NewPricingClient(connections["pricing"]),
		postgres.NewRepository(db),
		time.Now,
	)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	server := grpcx.NewServer(log)
	stocksyncv1.RegisterStockSyncServiceServer(server, grpcadapter.NewHandler(service))
	return server.Serve(ctx, fmt.Sprintf(":%d", port))
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
