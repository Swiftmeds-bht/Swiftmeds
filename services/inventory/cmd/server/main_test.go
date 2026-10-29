package main

import (
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
