// Command server runs the Substitution service.
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
	substitutionv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/substitution/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/adapters/clients"
	grpcadapter "github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/adapters/grpc"
	"github.com/Swiftmeds-bht/Swiftmeds/services/substitution/internal/app"
)

const (
	serviceName = "substitution"
	defaultPort = 50058
	portEnv     = "SUBSTITUTION_GRPC_PORT"
)

func main() {
	os.Exit(mainRun(os.Args[1:]))
}

func mainRun(args []string) int {
	if len(args) > 0 && args[0] == "healthcheck" {
		return healthcheck()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func run(ctx context.Context) error {
	cfg := config.New()
	port := cfg.Int(portEnv, defaultPort)
	upstream := cfg.String("SUBSTITUTION_UPSTREAM", "traefik:50051")
	callTimeout := cfg.Duration("SUBSTITUTION_CALL_TIMEOUT", 800*time.Millisecond)
	logLevel := cfg.String("LOG_LEVEL", "info")
	if err := cfg.Err(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := logger.New(serviceName, logLevel)

	catalogConn, err := grpc.NewClient(
		upstream,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority("catalog"),
		grpc.WithChainUnaryInterceptor(
			resilience.UnaryClientInterceptor(resilience.Config{
				Timeout: callTimeout,
				Retry: resilience.RetryConfig{
					MaxAttempts: 2,
					BaseDelay:   50 * time.Millisecond,
				},
			}),
		),
	)
	if err != nil {
		return fmt.Errorf("create catalog client: %w", err)
	}
	defer func() {
		_ = catalogConn.Close()
	}()

	service := app.NewService(clients.NewCatalogClient(catalogConn))

	server := grpcx.NewServer(log)
	substitutionv1.RegisterSubstitutionServiceServer(
		server,
		grpcadapter.NewHandler(service),
	)
	return server.Serve(ctx, fmt.Sprintf(":%d", port))
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
