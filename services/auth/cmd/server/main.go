package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/migrate"
	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/adapters/postgres"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/migrations"
)

const (
	serviceName = "auth"
	defaultPort = 50051
	portEnv     = "AUTH_GRPC_PORT"
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
	databaseURL := cfg.Required("AUTH_DATABASE_URL")
	secret := cfg.Required("JWT_SECRET")

	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if len([]byte(secret)) < 32 {
		return fmt.Errorf("config: JWT_SECRET must be at least 32 bytes")
	}
	if isPlaceholderSecret(secret) {
		return fmt.Errorf("config: JWT_SECRET must not be a placeholder")
	}
	issuer := cfg.String("JWT_ISSUER", "swiftmed")
	accessTTL := cfg.Duration("JWT_TTL", 15*time.Minute)
	refreshTTL := cfg.Duration("JWT_REFRESH_TTL", 30*24*time.Hour)
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}

	// Bring auth_db up to date before serving any request.
	sqlDB := stdlib.OpenDBFromPool(db)
	defer func() { _ = sqlDB.Close() }()
	applied, err := migrate.Up(ctx, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Info("database migrations up to date", slog.Int("applied", len(applied)), slog.Any("versions", applied))

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repo := postgres.NewRepository(db)
	svc := app.NewService(repo, []byte(secret), issuer, accessTTL, refreshTTL, time.Now)
	srv := grpcx.NewServer(log)
	authv1.RegisterAuthServiceServer(srv, grpcadapter.NewHandler(svc))
	return srv.Serve(signalCtx, fmt.Sprintf(":%d", port))
}

func isPlaceholderSecret(secret string) bool {
	value := strings.ToLower(strings.TrimSpace(secret))
	return strings.Contains(value, "change-me") || strings.Contains(value, "changeme") ||
		strings.Contains(value, "replace-with") || strings.Contains(value, "placeholder") ||
		value == "secret" || value == "password"
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
