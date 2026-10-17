// Command server runs the Pricing service.
//
// main only wires things together: config -> logger -> adapters -> app -> gRPC server.
// Run "server healthcheck" to check a running instance (used by the Docker HEALTHCHECK).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/app"
)

const (
	serviceName = "pricing"
	defaultPort = 50063
	portEnv     = "PRICING_GRPC_PORT"
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
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := logger.New(serviceName, logLevel)

	// Cancelled on Ctrl+C or when Docker stops the container (SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Outbound adapters -> use cases -> inbound adapter.
	repo := memory.NewRepository()
	svc := app.NewService(repo, time.Now)

	srv := grpcx.NewServer(log)
	pricingv1.RegisterPricingServiceServer(srv, grpcadapter.NewHandler(svc))

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
