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
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	notificationv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/notification/v1"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/adapters/noop"
	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/adapters/postgres"
	rabbitmqadapter "github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/adapters/rabbitmq"
	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/notification/migrations"
)

const (
	serviceName = "notification"
	defaultPort = 50054
	portEnv     = "NOTIFICATION_GRPC_PORT"
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
	databaseURL := cfg.Required("NOTIFICATION_DATABASE_URL")
	rabbitURL := cfg.Required("RABBITMQ_URL")
	logLevel := cfg.String("LOG_LEVEL", "info")
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.New(serviceName, logLevel)
	slog.SetDefault(log)

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
	if _, err := migrate.Up(startCtx, sqlDB, migrations.FS); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if err := messaging.EnsureSchema(startCtx, sqlDB); err != nil {
		return fmt.Errorf("messaging schema: %w", err)
	}

	rabbitConn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return fmt.Errorf("RabbitMQ connection: %w", err)
	}
	defer func() { _ = rabbitConn.Close() }()
	ch, err := rabbitConn.Channel()
	if err != nil {
		return fmt.Errorf("RabbitMQ consumer channel: %w", err)
	}
	defer func() { _ = ch.Close() }()
	deduper, err := messaging.NewPostgresDeduper(sqlDB)
	if err != nil {
		return fmt.Errorf("event deduper: %w", err)
	}
	svc := app.NewService(postgres.NewRepository(db), noop.Delivery{}, time.Now)
	consumer, err := rabbitmqadapter.NewConsumer(ch, deduper, svc, log)
	if err != nil {
		return err
	}

	srv := grpcx.NewServer(log)
	notificationv1.RegisterNotificationServiceServer(srv, grpcadapter.NewHandler(svc))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtimeErrors := make(chan error, 2)
	go func() { runtimeErrors <- srv.Serve(ctx, fmt.Sprintf(":%d", port)) }()
	go func() { runtimeErrors <- consumer.Start(ctx) }()
	first := <-runtimeErrors
	stop()
	second := <-runtimeErrors
	if ctx.Err() != nil {
		for _, runtimeErr := range []error{first, second} {
			if runtimeErr != nil && !errors.Is(runtimeErr, context.Canceled) {
				return runtimeErr
			}
		}
		return nil
	}
	if first != nil {
		return first
	}
	if second != nil {
		return second
	}
	return errors.New("notification runtime stopped unexpectedly")
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
