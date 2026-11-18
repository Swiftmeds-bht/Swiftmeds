// Command server runs the Watchlist service: the WatchlistService gRPC API and
// the stock.increased consumer that records back-in-stock alerts.
//
// main only wires things together: config -> logger -> adapters -> app -> runtimes.
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
	watchlistv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/adapters/clients"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/adapters/postgres"
	rabbitmqadapter "github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/adapters/rabbitmq"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/migrations"
)

const (
	serviceName = "watchlist"
	defaultPort = 50064
	portEnv     = "WATCHLIST_GRPC_PORT"
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
	databaseURL := cfg.Required("WATCHLIST_DATABASE_URL")
	rabbitURL := cfg.Required("RABBITMQ_URL")
	upstream := cfg.String("WATCHLIST_UPSTREAM", "traefik:50051")
	geoTimeout := cfg.Duration("WATCHLIST_GEO_TIMEOUT", 2*time.Second)
	logLevel := cfg.String("LOG_LEVEL", "info")
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := logger.New(serviceName, logLevel)
	// The event consumer logs through slog's default logger: make it this JSON one.
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

	// Bring watchlist_db up to date before serving any request.
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

	// Geo through Traefik, which routes by :authority. Retries are safe: the call only reads.
	geoConn, err := grpc.NewClient(upstream,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority("geo"),
		grpc.WithChainUnaryInterceptor(resilience.UnaryClientInterceptor(resilience.Config{
			Timeout: geoTimeout,
			Retry:   resilience.RetryConfig{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond},
		})),
	)
	if err != nil {
		return fmt.Errorf("geo client: %w", err)
	}
	defer func() { _ = geoConn.Close() }()

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
	messagePublisher, err := messaging.NewPublisher(publisherChannel)
	if err != nil {
		return fmt.Errorf("event publisher: %w", err)
	}
	alertPublisher := rabbitmqadapter.NewPublisher(messagePublisher)

	// Outbound adapters -> use cases -> inbound adapters.
	svc := app.NewService(postgres.NewRepository(db), clients.NewGeo(geoConn), time.Now, app.NewID, alertPublisher)

	deduper, err := messaging.NewPostgresDeduper(sqlDB)
	if err != nil {
		return fmt.Errorf("event deduper: %w", err)
	}
	stockConsumer, err := rabbitmqadapter.NewStockIncreasedConsumer(consumerChannel, deduper, svc, time.Now)
	if err != nil {
		return err
	}

	srv := grpcx.NewServer(log)
	watchlistv1.RegisterWatchlistServiceServer(srv, grpcadapter.NewHandler(svc))

	// Cancelled on Ctrl+C or when Docker stops the container (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtimeErrors := make(chan error, 2)
	go func() { runtimeErrors <- srv.Serve(ctx, fmt.Sprintf(":%d", port)) }()
	go func() { runtimeErrors <- stockConsumer.Start(ctx) }()
	return waitForRuntime(ctx, stop, runtimeErrors)
}

// waitForRuntime returns when the gRPC server or the consumer stops. If one
// fails, the other is stopped too, so the container exits and Docker restarts it.
func waitForRuntime(ctx context.Context, stop context.CancelFunc, runtimeErrors <-chan error) error {
	first := <-runtimeErrors
	shuttingDown := ctx.Err() != nil
	stop()
	second := <-runtimeErrors

	for _, err := range []error{first, second} {
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("watchlist runtime: %w", err)
		}
	}
	if !shuttingDown {
		return errors.New("watchlist runtime stopped unexpectedly")
	}
	return nil
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
