// Command server runs the Expiry service.
//
// main only wires things together: config -> logger -> postgres + migrations ->
// RabbitMQ -> Holds client -> app -> gRPC server and event consumer.
// Run "server healthcheck" to check a running instance (used by the Docker HEALTHCHECK).
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/resilience"
	expiryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/expiry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/adapters/clients"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/adapters/postgres"
	rabbitmqadapter "github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/adapters/rabbitmq"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/migrations"
)

const (
	serviceName = "expiry"
	defaultPort = 50066
	portEnv     = "EXPIRY_GRPC_PORT"
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
	databaseURL := cfg.Required("EXPIRY_DATABASE_URL")
	rabbitURL := cfg.Required("RABBITMQ_URL")
	holdsUpstream := cfg.String("EXPIRY_HOLDS_UPSTREAM", "traefik:50051")
	callTimeout := cfg.Duration("EXPIRY_HOLDS_CALL_TIMEOUT", 2*time.Second)
	holdDuration := cfg.Duration("EXPIRY_HOLD_DURATION", app.DefaultHoldDuration)
	batchSize := cfg.Int("EXPIRY_SWEEP_BATCH_SIZE", app.DefaultBatchSize)
	logLevel := cfg.String("LOG_LEVEL", "info")
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if holdDuration <= 0 {
		return fmt.Errorf("config: EXPIRY_HOLD_DURATION must be greater than zero, got %s", holdDuration)
	}
	if batchSize < 1 {
		return fmt.Errorf("config: EXPIRY_SWEEP_BATCH_SIZE must be at least 1, got %d", batchSize)
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

	// Bring expiry_db up to date before serving any request.
	sqlDB := stdlib.OpenDBFromPool(db)
	defer func() { _ = sqlDB.Close() }()
	applied, err := migrate.Up(startCtx, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Info("database migrations up to date", slog.Int("applied", len(applied)), slog.Any("versions", applied))

	if err := messaging.EnsureSchema(startCtx, sqlDB); err != nil {
		return fmt.Errorf("messaging deduplication schema: %w", err)
	}

	rabbitConn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return fmt.Errorf("RabbitMQ connection: %w", err)
	}
	defer func() { _ = rabbitConn.Close() }()
	consumerChannel, err := rabbitConn.Channel()
	if err != nil {
		return fmt.Errorf("RabbitMQ consumer channel: %w", err)
	}
	defer func() { _ = consumerChannel.Close() }()

	// Holds is reached through Traefik, which routes by the authority "holds".
	// Timeout and retry: ExpireHold is safe to repeat, so a retry after an
	// UNAVAILABLE answer cannot expire a hold twice.
	holdsConn, err := grpc.NewClient(
		holdsUpstream,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority("holds"),
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
	if err != nil {
		return fmt.Errorf("create holds client: %w", err)
	}
	defer func() { _ = holdsConn.Close() }()

	// Outbound adapters -> use cases -> inbound adapters.
	svc := app.NewService(
		postgres.NewRepository(db),
		clients.NewHoldsClient(holdsConn),
		app.Config{BatchSize: batchSize, HoldDuration: holdDuration},
		time.Now,
		log,
	)
	deduper, err := messaging.NewPostgresDeduper(sqlDB)
	if err != nil {
		return fmt.Errorf("create event deduper: %w", err)
	}
	holdConsumer, err := rabbitmqadapter.NewHoldConfirmedConsumer(consumerChannel, deduper, svc)
	if err != nil {
		return fmt.Errorf("create HoldConfirmed consumer: %w", err)
	}

	srv := grpcx.NewServer(log)
	expiryv1.RegisterExpiryServiceServer(srv, grpcadapter.NewHandler(svc))

	// Cancelled on Ctrl+C or when Docker stops the container (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtimeErrors := make(chan error, 2)
	go func() { runtimeErrors <- srv.Serve(ctx, fmt.Sprintf(":%d", port)) }()
	go func() { runtimeErrors <- holdConsumer.Start(ctx) }()

	return waitForRuntime(ctx, stop, runtimeErrors)
}

// waitForRuntime waits for the gRPC server and the event consumer. When one of
// them ends, it stops the other, and reports the first real error.
func waitForRuntime(ctx context.Context, stop context.CancelFunc, runtimeErrors <-chan error) error {
	firstErr := <-runtimeErrors
	if ctx.Err() != nil {
		// A normal shutdown: both parts end because the signal context ended.
		stop()
		secondErr := <-runtimeErrors
		if firstErr != nil && !errors.Is(firstErr, context.Canceled) {
			return fmt.Errorf("expiry runtime shutdown: %w", firstErr)
		}
		if secondErr != nil && !errors.Is(secondErr, context.Canceled) {
			return fmt.Errorf("expiry runtime shutdown: %w", secondErr)
		}
		return nil
	}

	// One part failed on its own: stop the other and report the failure.
	stop()
	secondErr := <-runtimeErrors
	if firstErr != nil {
		return fmt.Errorf("expiry runtime: %w", firstErr)
	}
	if secondErr != nil {
		return fmt.Errorf("expiry runtime: %w", secondErr)
	}
	return errors.New("expiry runtime stopped unexpectedly")
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
