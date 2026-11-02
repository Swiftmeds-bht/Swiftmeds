package main

import (
	"context"
	"net"
	"strconv"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

func TestHealthcheckWithServingServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := grpc.NewServer()
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthv1.HealthCheckResponse_SERVING)
	healthv1.RegisterHealthServer(server, healthServer)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	t.Setenv(portEnv, strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	if got := healthcheck(); got != 0 {
		t.Fatalf("healthcheck returned %d, want 0", got)
	}
}

func TestHealthcheckWhenServerIsUnavailable(t *testing.T) {
	t.Setenv(portEnv, "0")
	if got := healthcheck(); got != 1 {
		t.Fatalf("healthcheck returned %d, want 1", got)
	}
}

func TestMainRunHealthcheckCommand(t *testing.T) {
	t.Setenv(portEnv, "0")
	if got := mainRun([]string{"healthcheck"}); got != 1 {
		t.Fatalf("mainRun returned %d, want 1", got)
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("SUBSTITUTION_CALL_TIMEOUT", "not-a-duration")
	if err := run(context.Background()); err == nil {
		t.Fatal("run returned nil for invalid configuration")
	}
}

func TestRunStopsWhenContextIsCancelled(t *testing.T) {
	t.Setenv(portEnv, "0")
	t.Setenv("SUBSTITUTION_UPSTREAM", "127.0.0.1:1")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := run(ctx); err != nil {
		t.Fatalf("run returned error after cancellation: %v", err)
	}
}
