package grpcx

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// CheckHealth calls the standard gRPC health service at addr and returns nil
// only if the server reports SERVING.
//
// Services call this from a "healthcheck" sub-command so the Docker HEALTHCHECK
// can run the service binary itself; distroless images have no shell or curl.
func CheckHealth(ctx context.Context, addr string) error {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() {
		// A failure closing the probe's own connection says nothing about the
		// server's health, so it must not turn a SERVING result into an error.
		_ = conn.Close()
	}()

	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("health check %s: %w", addr, err)
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("health check %s: status %s", addr, resp.GetStatus())
	}
	return nil
}
