// Command server runs the Trust service: the TrustService gRPC API (GetTrust,
// BatchGetTrust) and the feedback.submitted consumer that keeps each
// pharmacy's trust score up to date.
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

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	trustv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/trust/v1"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/adapters/postgres"
	rabbitmqadapter "github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/adapters/rabbitmq"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/trust/migrations"
)

const (
	serviceName = "trust"
	defaultPort = 50068
	portEnv     = "TRUST_GRPC_PORT"
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
	databaseURL := cfg.Required("TRUST_DATABASE_URL")
	rabbitURL := cfg.Required("RABBITMQ_URL")
	logLevel := cfg.String("LOG_LEVEL", "info")
	params, err := app.ParamsFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := logger.New(serviceName, logLevel)
	// The event consumer logs through slog's default logger: make it this JSON one.
	slog.SetDefault(log)
	log.Info("trust score parameters",
		slog.String("half_life", params.HalfLife.String()), slog.Float64("prior_weight", params.PriorWeight))

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

	// Bring trust_db up to date before serving any request.
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

	rabbitConn, err := messaging.Dial(context.Background(), rabbitURL, log)
	if err != nil {
		return fmt.Errorf("RabbitMQ connection: %w", err)
	}
	defer func() { _ = rabbitConn.Close() }()
	consumerChannel, err := rabbitConn.Channel()
	if err != nil {
		return fmt.Errorf("RabbitMQ consumer channel: %w", err)
	}
	defer func() { _ = consumerChannel.Close() }()

	// Outbound adapters -> use cases -> inbound adapters.
	svc, err := app.NewService(postgres.NewRepository(db), params, time.Now)
	if err != nil {
		return err
	}

	deduper, err := messaging.NewPostgresDeduper(sqlDB)
	if err != nil {
		return fmt.Errorf("event deduper: %w", err)
	}
	feedbackConsumer, err := rabbitmqadapter.NewFeedbackSubmittedConsumer(consumerChannel, deduper, svc, time.Now)
	if err != nil {
		return err
	}

	srv := grpcx.NewServer(log)
	trustv1.RegisterTrustServiceServer(srv, grpcadapter.NewHandler(svc))

	// Cancelled on Ctrl+C or when Docker stops the container (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtimeErrors := make(chan error, 2)
	go func() { runtimeErrors <- srv.Serve(ctx, fmt.Sprintf(":%d", port)) }()
	go func() { runtimeErrors <- feedbackConsumer.Start(ctx) }()
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
			return fmt.Errorf("trust runtime: %w", err)
		}
	}
	if !shuttingDown {
		return errors.New("trust runtime stopped unexpectedly")
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
