// Command server runs the Feedback service: the FeedbackService gRPC API. It
// reads holds from the Holds service and publishes feedback.submitted.
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
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/resilience"
	feedbackv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/feedback/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/adapters/clients"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/adapters/postgres"
	rabbitmqadapter "github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/adapters/rabbitmq"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/migrations"
)

const (
	serviceName = "feedback"
	defaultPort = 50067
	portEnv     = "FEEDBACK_GRPC_PORT"
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
	databaseURL := cfg.Required("FEEDBACK_DATABASE_URL")
	rabbitURL := cfg.Required("RABBITMQ_URL")
	upstream := cfg.String("FEEDBACK_UPSTREAM", "traefik:50051")
	holdsTimeout := cfg.Duration("FEEDBACK_HOLDS_TIMEOUT", 2*time.Second)
	logLevel := cfg.String("LOG_LEVEL", "info")
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if holdsTimeout <= 0 {
		return fmt.Errorf("config: FEEDBACK_HOLDS_TIMEOUT must be greater than zero, got %s", holdsTimeout)
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

	// Bring feedback_db up to date before serving any request.
	sqlDB := stdlib.OpenDBFromPool(db)
	defer func() { _ = sqlDB.Close() }()
	applied, err := migrate.Up(startCtx, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Info("database migrations up to date", slog.Int("applied", len(applied)), slog.Any("versions", applied))

	// Holds through Traefik, which routes by :authority. Retries are safe: the call only reads.
	holdsConn, err := grpc.NewClient(upstream,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority("holds"),
		grpc.WithChainUnaryInterceptor(resilience.UnaryClientInterceptor(resilience.Config{
			Timeout: holdsTimeout,
			Retry:   resilience.RetryConfig{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond},
		})),
	)
	if err != nil {
		return fmt.Errorf("holds client: %w", err)
	}
	defer func() { _ = holdsConn.Close() }()

	rabbitConn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return fmt.Errorf("RabbitMQ connection: %w", err)
	}
	defer func() { _ = rabbitConn.Close() }()
	publisherChannel, err := rabbitConn.Channel()
	if err != nil {
		return fmt.Errorf("RabbitMQ publisher channel: %w", err)
	}
	defer func() { _ = publisherChannel.Close() }()
	events, err := rabbitmqadapter.NewPublisher(publisherChannel)
	if err != nil {
		return err
	}

	// Outbound adapters -> use cases -> inbound adapter.
	svc := app.NewService(postgres.NewRepository(db), clients.NewHolds(holdsConn), events, time.Now)

	srv := grpcx.NewServer(log)
	feedbackv1.RegisterFeedbackServiceServer(srv, grpcadapter.NewHandler(svc))

	// Cancelled on Ctrl+C or when Docker stops the container (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
