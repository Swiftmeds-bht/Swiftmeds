package main

import (
	"context"
	"errors"
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
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	inventoryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/inventory/v1"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/adapters/postgres"
	rabbitmqadapter "github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/adapters/rabbitmq"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/inventory/migrations"
)

const (
	serviceName = "inventory"
	defaultPort = 50056
	portEnv     = "INVENTORY_GRPC_PORT"
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
	databaseURL := cfg.Required("INVENTORY_DATABASE_URL")
	rabbitURL := cfg.Required("RABBITMQ_URL")
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
	defer func() { _ = sqlDB.Close() }()

	applied, err := migrate.Up(startCtx, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Info(
		"database migrations up to date",
		slog.Int("applied", len(applied)),
		slog.Any("versions", applied),
	)

	if err := messaging.EnsureSchema(startCtx, sqlDB); err != nil {
		return fmt.Errorf("messaging deduplication schema: %w", err)
	}

	rabbitConn, err := messaging.Dial(context.Background(), rabbitURL, log)
	if err != nil {
		return fmt.Errorf("RabbitMQ connection: %w", err)
	}
	defer func() { _ = rabbitConn.Close() }()

	publisherChannel, err := rabbitConn.Channel()
	if err != nil {
		return fmt.Errorf("RabbitMQ publisher channel: %w", err)
	}
	defer func() { _ = publisherChannel.Close() }()

	consumerChannel, err := rabbitConn.Channel()
	if err != nil {
		return fmt.Errorf("RabbitMQ consumer channel: %w", err)
	}
	defer func() { _ = consumerChannel.Close() }()

	repo, err := postgres.NewRepository(db)
	if err != nil {
		return fmt.Errorf("create inventory repository: %w", err)
	}

	eventPublisher, err := rabbitmqadapter.NewPublisher(publisherChannel)
	if err != nil {
		return fmt.Errorf("create inventory event publisher: %w", err)
	}

	service, err := app.NewService(repo, eventPublisher, time.Now)
	if err != nil {
		return fmt.Errorf("create inventory application service: %w", err)
	}

	grpcHandler, err := grpcadapter.NewServer(service)
	if err != nil {
		return fmt.Errorf("create Inventory gRPC handler: %w", err)
	}

	deduper, err := messaging.NewPostgresDeduper(sqlDB)
	if err != nil {
		return fmt.Errorf("create event deduper: %w", err)
	}

	holdConsumer, err := rabbitmqadapter.NewHoldRequestedConsumer(
		consumerChannel,
		deduper,
		service,
	)
	if err != nil {
		return fmt.Errorf("create HoldRequested consumer: %w", err)
	}

	grpcServer := grpcx.NewServer(log)
	inventoryv1.RegisterInventoryServiceServer(grpcServer, grpcHandler)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	runtimeErrors := make(chan error, 2)
	go func() {
		runtimeErrors <- grpcServer.Serve(ctx, fmt.Sprintf(":%d", port))
	}()
	go func() {
		runtimeErrors <- holdConsumer.Start(ctx)
	}()

	return waitForRuntime(ctx, stop, runtimeErrors)
}

func waitForRuntime(
	ctx context.Context,
	stop context.CancelFunc,
	runtimeErrors <-chan error,
) error {
	firstErr := <-runtimeErrors
	if ctx.Err() != nil {
		stop()
		secondErr := <-runtimeErrors

		if firstErr != nil && !errors.Is(firstErr, context.Canceled) {
			return fmt.Errorf("inventory runtime shutdown: %w", firstErr)
		}
		if secondErr != nil && !errors.Is(secondErr, context.Canceled) {
			return fmt.Errorf("inventory runtime shutdown: %w", secondErr)
		}
		return nil
	}

	stop()
	secondErr := <-runtimeErrors
	if firstErr != nil {
		return fmt.Errorf("inventory runtime: %w", firstErr)
	}
	if secondErr != nil {
		return fmt.Errorf("inventory runtime: %w", secondErr)
	}
	return errors.New("inventory runtime stopped unexpectedly")
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
