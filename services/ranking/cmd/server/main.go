// Command server runs the Ranking service: pure computation, no database.
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

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	rankingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/ranking/v1"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/ranking/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/ranking/internal/app"
)

const (
	serviceName = "ranking"
	defaultPort = 50061
	portEnv     = "RANKING_GRPC_PORT"
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
	logLevel := cfg.String("LOG_LEVEL", "info")
	weights, err := app.WeightsFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := logger.New(serviceName, logLevel)
	svc, err := app.NewService(weights)
	if err != nil {
		return err
	}
	log.Info("default ranking weights",
		slog.Float64("distance", weights.Distance), slog.Float64("freshness", weights.Freshness),
		slog.Float64("trust", weights.Trust), slog.Float64("price", weights.Price))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := grpcx.NewServer(log)
	rankingv1.RegisterRankingServiceServer(srv, grpcadapter.NewHandler(svc))
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
