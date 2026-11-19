package grpcx

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func TestConnectionRequiresServiceName(t *testing.T) {
	f := NewClientFactory("localhost:1")
	t.Cleanup(func() { _ = f.Close() })

	if _, err := f.Connection(context.Background(), ""); err == nil {
		t.Fatal("Connection(\"\") returned nil error, want error")
	}
}

func TestConnectionIsLazyAndCachedPerService(t *testing.T) {
	// Nothing listens on this port: NewClient must not dial until the first RPC.
	f := NewClientFactory("localhost:1")
	t.Cleanup(func() { _ = f.Close() })

	auth1, err := f.Connection(context.Background(), "auth")
	if err != nil {
		t.Fatalf("Connection(auth) error = %v, want nil (connection is lazy)", err)
	}
	auth2, err := f.Connection(context.Background(), "auth")
	if err != nil {
		t.Fatal(err)
	}
	if auth1 != auth2 {
		t.Error("second Connection(auth) returned a new conn, want the cached one")
	}

	catalog, err := f.Connection(context.Background(), "catalog")
	if err != nil {
		t.Fatal(err)
	}
	if catalog == auth1 {
		t.Error("Connection(catalog) returned the auth conn, want one per service")
	}
}

func TestCloseClearsCache(t *testing.T) {
	f := NewClientFactory("localhost:1")

	first, err := f.Connection(context.Background(), "auth")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	second, err := f.Connection(context.Background(), "auth")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if first == second {
		t.Error("Connection after Close returned the closed conn, want a new one")
	}
}

func TestConnectionWithTimeoutRejectsBadInput(t *testing.T) {
	f := NewClientFactory("localhost:1")
	t.Cleanup(func() { _ = f.Close() })

	if _, err := f.ConnectionWithTimeout(context.Background(), "", time.Second); err == nil {
		t.Error("empty service name: err = nil, want an error")
	}
	for _, timeout := range []time.Duration{0, -time.Second} {
		if _, err := f.ConnectionWithTimeout(context.Background(), "expiry", timeout); err == nil {
			t.Errorf("timeout %v: err = nil, want an error", timeout)
		}
	}
}

// slowHealthServer starts a gRPC server whose every call takes delay, and
// returns its address.
func slowHealthServer(t *testing.T, delay time.Duration) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	slow := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		time.Sleep(delay)
		return handler(ctx, req)
	}
	srv := grpc.NewServer(grpc.UnaryInterceptor(slow))
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func TestColdSearchCallFitsUpstreamDeadline(t *testing.T) {
	// The first call also establishes the lazy gRPC connection. Search's upstream
	// budget is 2 seconds, so its per-attempt limit must allow a 1 second cold call.
	f := NewClientFactory(slowHealthServer(t, time.Second))
	t.Cleanup(func() { _ = f.Close() })
	search, err := f.ConnectionWithTimeout(context.Background(), "search", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := healthpb.NewHealthClient(search).Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatalf("cold 1 second Search call under 2 second deadline failed: %v (code %v)", err, status.Code(err))
	}
}
