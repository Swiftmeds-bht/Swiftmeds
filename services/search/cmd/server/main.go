// Command server runs the Search service. It owns no data: it calls Catalog,
// Geo, Inventory, Freshness, Pricing, Trust, Pharmacy Registry, Ranking and
// Substitution through Traefik, each with its own resilience interceptor.
// Run "server healthcheck" to check a running instance (used by the Docker HEALTHCHECK).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/config"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/grpcx"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/logger"
	"github.com/Swiftmeds-bht/Swiftmeds/pkg/resilience"
	searchv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/search/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/adapters/clients"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/search/internal/app"
)

const (
	serviceName = "search"
	defaultPort = 50060
	portEnv     = "SEARCH_GRPC_PORT"
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
	upstream := cfg.String("SEARCH_UPSTREAM", "traefik:50051")
	callTimeout := cfg.Duration("SEARCH_CALL_TIMEOUT", 800*time.Millisecond)
	requestTimeout := cfg.Duration("SEARCH_REQUEST_TIMEOUT", 2*time.Second)
	logLevel := cfg.String("LOG_LEVEL", "info")
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.New(serviceName, logLevel)

	// One connection per downstream service: Traefik routes by :authority, and
	// each connection gets its own breaker and bulkhead. Retries are limited to
	// one extra attempt so a slow service cannot eat the 2 s search budget.
	dial := func(authority string) (*grpc.ClientConn, error) {
		return grpc.NewClient(upstream,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithAuthority(authority),
			grpc.WithChainUnaryInterceptor(resilience.UnaryClientInterceptor(resilience.Config{
				Timeout: callTimeout,
				Retry:   resilience.RetryConfig{MaxAttempts: 2, BaseDelay: 50 * time.Millisecond},
			})),
		)
	}
	conns := map[string]*grpc.ClientConn{}
	for _, name := range []string{"catalog", "geo", "inventory", "freshness", "pricing", "trust", "pharmacy-registry", "ranking", "substitution"} {
		c, err := dial(name)
		if err != nil {
			return fmt.Errorf("client for %s: %w", name, err)
		}
		defer func() { _ = c.Close() }()
		conns[name] = c
	}

	svc := app.NewService(app.Deps{
		Catalog:      clients.NewCatalog(conns["catalog"]),
		Geo:          clients.NewGeo(conns["geo"]),
		Inventory:    clients.NewInventory(conns["inventory"]),
		Freshness:    clients.NewFreshness(conns["freshness"]),
		Pricing:      clients.NewPricing(conns["pricing"]),
		Trust:        clients.NewTrust(conns["trust"]),
		Registry:     clients.NewRegistry(conns["pharmacy-registry"]),
		Ranking:      clients.NewRanking(conns["ranking"]),
		Substitution: clients.NewSubstitution(conns["substitution"]),
	}, app.Options{CallTimeout: callTimeout, RequestTimeout: requestTimeout, Logger: log})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := grpcx.NewServer(log)
	searchv1.RegisterSearchServiceServer(srv, grpcadapter.NewHandler(svc))
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
