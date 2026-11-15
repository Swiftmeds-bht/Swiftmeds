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
	analyticsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/analytics/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/adapters/clients"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/adapters/postgres"
	rabbitadapter "github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/adapters/rabbitmq"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/analytics/migrations"
)

const (
	portEnv     = "ANALYTICS_GRPC_PORT"
	defaultPort = 50059
)

func main() {
	// The image is distroless (no shell or curl), so Docker's HEALTHCHECK runs
	// "/server healthcheck", which asks this server's gRPC health service.
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
	dbURL := cfg.Required("ANALYTICS_DATABASE_URL")
	rabbitURL := cfg.Required("RABBITMQ_URL")
	upstream := cfg.String("ANALYTICS_UPSTREAM", "traefik:50051")
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.New("analytics", cfg.String("LOG_LEVEL", "info"))
	slog.SetDefault(log)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(db)
	defer func() { _ = sqlDB.Close() }()
	if _, err := migrate.Up(ctx, sqlDB, migrations.FS); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if err := messaging.EnsureSchema(ctx, sqlDB); err != nil {
		return fmt.Errorf("deduplication schema: %w", err)
	}

	catalogConn, err := grpc.NewClient(upstream, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithAuthority("catalog"),
		grpc.WithChainUnaryInterceptor(resilience.UnaryClientInterceptor(resilience.Config{Timeout: 2 * time.Second})))
	if err != nil {
		return fmt.Errorf("catalog client: %w", err)
	}
	defer func() { _ = catalogConn.Close() }()
	registryConn, err := grpc.NewClient(upstream, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithAuthority("pharmacy-registry"),
		grpc.WithChainUnaryInterceptor(resilience.UnaryClientInterceptor(resilience.Config{Timeout: 2 * time.Second})))
	if err != nil {
		return fmt.Errorf("registry client: %w", err)
	}
	defer func() { _ = registryConn.Close() }()
	rabbitConn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return fmt.Errorf("RabbitMQ: %w", err)
	}
	defer func() { _ = rabbitConn.Close() }()
	ch, err := rabbitConn.Channel()
	if err != nil {
		return fmt.Errorf("RabbitMQ channel: %w", err)
	}
	defer func() { _ = ch.Close() }()

	svc := app.NewService(postgres.NewRepository(db), clients.NewRegistry(registryConn), clients.NewCatalog(catalogConn), time.Now)
	deduper, err := messaging.NewPostgresDeduper(sqlDB)
	if err != nil {
		return fmt.Errorf("deduper: %w", err)
	}
	consumer, err := rabbitadapter.NewConsumer(ch, deduper, svc, log)
	if err != nil {
		return err
	}
	srv := grpcx.NewServer(log)
	analyticsv1.RegisterAnalyticsServiceServer(srv, grpcadapter.NewHandler(svc))
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 2)
	go func() { errs <- srv.Serve(runCtx, fmt.Sprintf(":%d", port)) }()
	go func() { errs <- consumer.Start(runCtx) }()
	first := <-errs
	stop()
	second := <-errs
	for _, e := range []error{first, second} {
		if e != nil && !errors.Is(e, context.Canceled) {
			return e
		}
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
