package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/adapters/postgres"
	"github.com/Swiftmeds-bht/Swiftmeds/services/auth/internal/app"
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
	issuer := cfg.String("JWT_ISSUER", "swiftmed")
	accessTTL := cfg.Duration("JWT_TTL", 15*time.Minute)
	refreshTTL := cfg.Duration("JWT_REFRESH_TTL", 30*24*time.Hour)
	logLevel := cfg.String("LOG_LEVEL", "info")
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	db, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}

	log := logger.New(serviceName, logLevel)
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repo := postgres.NewRepository(db)
	svc := app.NewService(repo, []byte(secret), issuer, accessTTL, refreshTTL, time.Now)
	srv := grpcx.NewServer(log)
	authv1.RegisterAuthServiceServer(srv, grpcadapter.NewHandler(svc))
	return srv.Serve(signalCtx, fmt.Sprintf(":%d", port))
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
