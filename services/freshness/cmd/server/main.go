// Command server runs the Freshness service.
//
// main only wires things together: config -> logger -> database -> app -> gRPC server.
// Run "server healthcheck" to check a running instance (used by the Docker HEALTHCHECK).
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	freshnessv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/freshness/v1"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/adapters/postgres"
	rabbitmqadapter "github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/adapters/rabbitmq"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/internal/domain"
	"github.com/Swiftmeds-bht/Swiftmeds/services/freshness/migrations"
)

const (
	serviceName = "freshness"
	defaultPort = 50069
	portEnv     = "FRESHNESS_GRPC_PORT"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(config.New().Int(portEnv, defaultPort)))
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	s, err := loadSettings(config.New())
	if err != nil {
		return err
	}

	// Cancelled on Ctrl+C or when Docker stops the container (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	return serve(ctx, s, lis)
}

// serve connects to the database, runs the migrations and answers gRPC calls on lis
// until ctx is cancelled.
func serve(ctx context.Context, s settings, lis net.Listener) error {
	log := logger.New(serviceName, s.logLevel)

	// Outbound adapters -> use cases -> inbound adapter.
	db, err := sql.Open("pgx", s.databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	if err := postgres.Migrate(ctx, db, migrations.FS); err != nil {
		return err
	}
	if err := messaging.EnsureSchema(ctx, db); err != nil {
		return fmt.Errorf("messaging deduplication schema: %w", err)
	}

	svc := app.NewService(postgres.NewRepository(db), s.thresholds, time.Now)

	srv := grpcx.NewServer(log)
	freshnessv1.RegisterFreshnessServiceServer(srv, grpcadapter.NewHandler(svc))
	rabbitConn, err := messaging.Dial(ctx, s.rabbitURL, log)
	if err != nil {
		return fmt.Errorf("RabbitMQ connection: %w", err)
	}
	defer func() { _ = rabbitConn.Close() }()
	consumerChannel, err := rabbitConn.Channel()
	if err != nil {
		return fmt.Errorf("RabbitMQ consumer channel: %w", err)
	}
	defer func() { _ = consumerChannel.Close() }()
	deduper, err := messaging.NewPostgresDeduper(db)
	if err != nil {
		return fmt.Errorf("create event deduper: %w", err)
	}
	stockConsumer, err := rabbitmqadapter.NewStockIncreasedConsumer(consumerChannel, deduper, svc, time.Now)
	if err != nil {
		return err
	}

	log.Info("starting",
		"port", lis.Addr().(*net.TCPAddr).Port,
		"high_within", s.thresholds.HighWithin.String(),
		"medium_within", s.thresholds.MediumWithin.String())
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runtimeErrors := make(chan error, 2)
	go func() { runtimeErrors <- srv.ServeListener(serveCtx, lis) }()
	go func() { runtimeErrors <- stockConsumer.Start(serveCtx) }()
	first := <-runtimeErrors
	cancel()
	second := <-runtimeErrors
	if first != nil {
		return first
	}
	return second
}

// settings are the values read from environment variables.
type settings struct {
	port        int
	databaseURL string
	rabbitURL   string
	thresholds  domain.Thresholds
	logLevel    string
}

// loadSettings reads the settings and reports every problem at once.
func loadSettings(cfg *config.Loader) (settings, error) {
	def := domain.DefaultThresholds()
	s := settings{
		port:        cfg.Int(portEnv, defaultPort),
		databaseURL: cfg.Required("FRESHNESS_DATABASE_URL"),
		rabbitURL:   cfg.Required("RABBITMQ_URL"),
		logLevel:    cfg.String("LOG_LEVEL", "info"),
	}
	high := cfg.Duration("FRESHNESS_HIGH_WITHIN", def.HighWithin)
	medium := cfg.Duration("FRESHNESS_MEDIUM_WITHIN", def.MediumWithin)
	if err := cfg.Err(); err != nil {
		return settings{}, fmt.Errorf("config: %w", err)
	}

	thresholds, err := domain.NewThresholds(high, medium)
	if err != nil {
		return settings{}, fmt.Errorf("config: FRESHNESS_HIGH_WITHIN and FRESHNESS_MEDIUM_WITHIN: %w", err)
	}
	s.thresholds = thresholds
	return s, nil
}

// healthcheck returns 0 if the service on localhost:port reports healthy, otherwise 1.
func healthcheck(port int) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := grpcx.CheckHealth(ctx, fmt.Sprintf("localhost:%d", port)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
