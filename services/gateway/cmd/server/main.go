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
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/app"
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

	server := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           app.NewRouter(cfg, log),
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
