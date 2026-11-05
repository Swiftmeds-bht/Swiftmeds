// Command server runs the REST API gateway.
// Run "server healthcheck" to check a running instance (used by the Docker HEALTHCHECK).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	freshnessv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/freshness/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/adapters/clients"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/grpcx"
)

const serviceName = "gateway"

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
	cfg, err := app.LoadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.New(serviceName, cfg.LogLevel)
	if cfg.WorkerSecret == "" {
		log.Warn("WORKER_SHARED_SECRET is not set: internal worker endpoints reject every call")
	}

	// gRPC connections to other services go through Traefik. They connect
	// lazily, so the gateway starts even when Freshness is not up yet.
	conns := grpcx.NewClientFactory(cfg.TraefikTarget)
	defer func() {
		if err := conns.Close(); err != nil {
			log.Error("closing gRPC connections failed", slog.String("error", err.Error()))
		}
	}()
	freshnessConn, err := conns.Connection(context.Background(), "freshness")
	if err != nil {
		return fmt.Errorf("freshness client: %w", err)
	}
	freshness := clients.NewFreshnessClient(freshnessv1.NewFreshnessServiceClient(freshnessConn))
	authConn, err := conns.Connection(context.Background(), "auth")
	if err != nil {
		return fmt.Errorf("auth client: %w", err)
	}
	auth := clients.NewAuthClient(authv1.NewAuthServiceClient(authConn))

	server := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           app.NewRouter(cfg, log, app.WithFreshness(freshness), app.WithAuth(auth)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
		// No WriteTimeout: it would cut off long-lived responses such as the
		// planned watchlist alert stream. Handlers bound their own gRPC calls.
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error("gateway shutdown failed", slog.String("error", err.Error()))
		}
	}()

	log.Info("gateway listening", slog.String("addr", server.Addr))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("gateway stopped: %w", err)
	}
	log.Info("gateway stopped")
	return nil
}

func healthcheck() int {
	port := config.New().Int("GATEWAY_HTTP_PORT", app.DefaultHTTPPort)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://localhost:%d/healthz", port)
	if err := app.CheckHealth(ctx, http.DefaultClient, url); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
