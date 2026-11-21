// Command server runs the Holds service.
//
// main only wires things together: config -> logger -> adapters -> app -> gRPC server
// and the RabbitMQ consumer of the reservation saga.
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/resilience"
	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/clients"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/postgres"
	rabbitmqadapter "github.com/Swiftmeds-bht/Swiftmeds/services/holds/internal/adapters/rabbitmq"
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
	holdDuration := cfg.Duration("HOLDS_HOLD_DURATION", app.DefaultHoldDuration())
	pendingTimeout := cfg.Duration("HOLDS_PENDING_TIMEOUT", app.DefaultPendingTimeout())
	sweepInterval := cfg.Duration("HOLDS_SWEEP_INTERVAL", time.Minute)
	rabbitURL := cfg.Required("RABBITMQ_URL")
	// Calls to Inventory go through Traefik, which routes by :authority (the service name).
	upstream := cfg.String("HOLDS_UPSTREAM", "traefik:50051")
	callTimeout := cfg.Duration("HOLDS_CALL_TIMEOUT", 3*time.Second)
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
	if pendingTimeout <= 0 || sweepInterval <= 0 {
		return fmt.Errorf("config: HOLDS_PENDING_TIMEOUT (%s) and HOLDS_SWEEP_INTERVAL (%s) must be greater than zero",
			pendingTimeout, sweepInterval)
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

	// The table that remembers which event IDs were already handled.
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

	// Release and commit are safe to repeat, so the client may retry them.
	inventoryConn, err := grpc.NewClient(upstream,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority("inventory"),
		grpc.WithChainUnaryInterceptor(resilience.UnaryClientInterceptor(resilience.Config{
			Timeout: callTimeout,
			Retry:   resilience.RetryConfig{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond},
		})),
	)
	if err != nil {
		return fmt.Errorf("inventory client: %w", err)
	}
	defer func() { _ = inventoryConn.Close() }()

	messagePublisher, err := messaging.NewPublisher(publisherChannel)
	if err != nil {
		return fmt.Errorf("event publisher: %w", err)
	}
	eventPublisher, err := rabbitmqadapter.NewPublisher(messagePublisher)
	if err != nil {
		return fmt.Errorf("holds event publisher: %w", err)
	}
	deduper, err := messaging.NewPostgresDeduper(sqlDB)
	if err != nil {
		return fmt.Errorf("event deduper: %w", err)
	}

	// Outbound adapters -> use cases -> inbound adapters.
	repo := postgres.NewRepository(db)
	svc := app.NewService(repo, app.Config{
		MaxActivePerUser: maxActive, HoldDuration: holdDuration, PendingTimeout: pendingTimeout,
	}, time.Now,
		app.WithInventory(clients.NewInventory(inventoryConn)),
		app.WithEvents(eventPublisher))
	log.Info("holds settings",
		slog.Int("max_active_per_user", maxActive),
		slog.Duration("hold_duration", holdDuration),
		slog.Duration("pending_timeout", pendingTimeout),
		slog.Duration("sweep_interval", sweepInterval),
		slog.String("upstream", upstream))

	stockConsumer, err := rabbitmqadapter.NewStockConsumer(consumerChannel, deduper, svc, log)
	if err != nil {
		return fmt.Errorf("stock event consumer: %w", err)
	}

	srv := grpcx.NewServer(log)
	holdsv1.RegisterHoldsServiceServer(srv, grpcadapter.NewHandler(svc))

	// Cancelled on Ctrl+C or when Docker stops the container (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtimeErrors := make(chan error, 2)
	go func() { runtimeErrors <- srv.Serve(ctx, fmt.Sprintf(":%d", port)) }()
	go func() { runtimeErrors <- stockConsumer.Start(ctx) }()
	go sweepPeriodically(ctx, svc, sweepInterval, log)
	return waitForRuntime(ctx, stop, runtimeErrors)
}

// sweeper is the part of the service the periodic sweep needs.
type sweeper interface {
	SweepStaleHolds(ctx context.Context) (app.SweepResult, error)
}

// sweepPeriodically recovers holds the saga lost (app.SweepStaleHolds) every
// interval until ctx is cancelled. A failed sweep is logged and tried again on
// the next tick; it never stops the service.
func sweepPeriodically(ctx context.Context, s sweeper, interval time.Duration, log *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			res, err := s.SweepStaleHolds(ctx)
			if err != nil {
				log.Warn("stale hold sweep had failures", slog.Any("error", err),
					slog.Int("rejected", res.Rejected), slog.Int("released", res.Released),
					slog.Int("release_failed", res.ReleaseFailed), slog.Int("gave_up", res.GaveUp))
			} else if res.Rejected+res.Released+res.GaveUp > 0 {
				log.Info("stale holds recovered",
					slog.Int("rejected", res.Rejected), slog.Int("released", res.Released))
			}
		}
	}
}

// waitForRuntime waits for the gRPC server and the event consumer. If either one
// stops, it stops the other too, so the container restarts instead of running
// half alive (for example answering calls but no longer reading events).
func waitForRuntime(ctx context.Context, stop context.CancelFunc, runtimeErrors <-chan error) error {
	firstErr := <-runtimeErrors
	if ctx.Err() != nil {
		stop()
		secondErr := <-runtimeErrors
		for _, err := range []error{firstErr, secondErr} {
			if err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("holds runtime shutdown: %w", err)
			}
		}
		return nil
	}

	stop()
	secondErr := <-runtimeErrors
	if firstErr != nil {
		return fmt.Errorf("holds runtime: %w", firstErr)
	}
	if secondErr != nil {
		return fmt.Errorf("holds runtime: %w", secondErr)
	}
	return errors.New("holds runtime stopped unexpectedly")
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
