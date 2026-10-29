package main

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestRunRejectsMissingConfiguration(t *testing.T) {
	t.Setenv("INVENTORY_DATABASE_URL", "")
	t.Setenv("RABBITMQ_URL", "")

	err := run()
	if err == nil || !strings.HasPrefix(err.Error(), "config:") {
		t.Fatalf("run error = %v, want config error", err)
	}
}

func TestRunRejectsInvalidDatabaseURL(t *testing.T) {
	t.Setenv("INVENTORY_DATABASE_URL", "not-a-postgres-url")
	t.Setenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	t.Setenv("LOG_LEVEL", "error")

	err := run()
	if err == nil || !strings.HasPrefix(err.Error(), "postgres:") {
		t.Fatalf("run error = %v, want postgres configuration error", err)
	}
}

func TestRunReturnsPostgresPingError(t *testing.T) {
	t.Setenv(
		"INVENTORY_DATABASE_URL",
		"postgres://user:pass@127.0.0.1:1/inventory?connect_timeout=1",
	)
	t.Setenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	t.Setenv("LOG_LEVEL", "error")

	err := run()
	if err == nil || !strings.HasPrefix(err.Error(), "postgres ping:") {
		t.Fatalf("run error = %v, want postgres ping error", err)
	}
}

func TestHealthcheckReturnsSuccess(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	t.Setenv(portEnv, strconv.Itoa(port))

	server := grpc.NewServer()
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(serviceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, healthServer)

	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	if got := healthcheck(); got != 0 {
		t.Fatalf("healthcheck() = %d, want 0", got)
	}
}

func TestHealthcheckReturnsFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	t.Setenv(portEnv, strconv.Itoa(port))

	if got := healthcheck(); got != 1 {
		t.Fatalf("healthcheck() = %d, want 1", got)
	}
}

func TestWaitForRuntime(t *testing.T) {
	runtimeFailure := errors.New("runtime failure")

	tests := []struct {
		name          string
		cancelFirst   bool
		runtimeErrors []error
		wantError     error
		wantMessage   string
	}{
		{
			name:          "clean signal shutdown",
			cancelFirst:   true,
			runtimeErrors: []error{nil, nil},
		},
		{
			name:          "signal shutdown ignores cancellation errors",
			cancelFirst:   true,
			runtimeErrors: []error{context.Canceled, context.Canceled},
		},
		{
			name:          "signal shutdown reports first unexpected error",
			cancelFirst:   true,
			runtimeErrors: []error{runtimeFailure, nil},
			wantError:     runtimeFailure,
		},
		{
			name:          "signal shutdown reports second unexpected error",
			cancelFirst:   true,
			runtimeErrors: []error{context.Canceled, runtimeFailure},
			wantError:     runtimeFailure,
		},
		{
			name:          "runtime reports first error",
			runtimeErrors: []error{runtimeFailure, nil},
			wantError:     runtimeFailure,
		},
		{
			name:          "runtime reports second error",
			runtimeErrors: []error{nil, runtimeFailure},
			wantError:     runtimeFailure,
		},
		{
			name:          "both runtimes stop unexpectedly",
			runtimeErrors: []error{nil, nil},
			wantMessage:   "inventory runtime stopped unexpectedly",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.cancelFirst {
				cancel()
			}

			runtimeErrors := make(chan error, len(test.runtimeErrors))
			for _, err := range test.runtimeErrors {
				runtimeErrors <- err
			}

			got := waitForRuntime(ctx, cancel, runtimeErrors)
			if ctx.Err() == nil {
				t.Fatal("waitForRuntime did not cancel the context")
			}

			switch {
			case test.wantError != nil:
				if !errors.Is(got, test.wantError) {
					t.Fatalf("waitForRuntime error = %v, want %v", got, test.wantError)
				}
			case test.wantMessage != "":
				if got == nil || got.Error() != test.wantMessage {
					t.Fatalf("waitForRuntime error = %v, want %q", got, test.wantMessage)
				}
			case got != nil:
				t.Fatalf("waitForRuntime returned unexpected error: %v", got)
			}
		})
	}
}
