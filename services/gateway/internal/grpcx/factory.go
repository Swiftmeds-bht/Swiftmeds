package grpcx

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/resilience"
)

// DefaultCallTimeout is the limit for each attempt of a call on a connection
// made with Connection.
const DefaultCallTimeout = 800 * time.Millisecond

type ClientFactory struct {
	target string
	mu     sync.Mutex
	conns  map[string]*grpc.ClientConn
}

func NewClientFactory(traefikTarget string) *ClientFactory {
	return &ClientFactory{
		target: traefikTarget,
		conns:  make(map[string]*grpc.ClientConn),
	}
}

// Connection returns a connection through Traefik. service is the Traefik service
// name, for example "auth" or "catalog". Each attempt of a call may take up to
// DefaultCallTimeout.
//
// grpc.NewClient connects lazily: the TCP connection is made on the first RPC,
// so an unreachable Traefik surfaces as Unavailable on that call (bounded by the
// call's own deadline) instead of here.
func (f *ClientFactory) Connection(ctx context.Context, service string) (*grpc.ClientConn, error) {
	return f.ConnectionWithTimeout(ctx, service, DefaultCallTimeout)
}

// ConnectionWithTimeout is Connection with its own limit for each attempt of a
// call, for the few routes that do more work than an ordinary request (the
// hold-expiry sweep). The limit applies to the first connection made for a
// service; later calls for the same service return that connection.
func (f *ClientFactory) ConnectionWithTimeout(_ context.Context, service string, timeout time.Duration) (*grpc.ClientConn, error) {
	if service == "" {
		return nil, fmt.Errorf("gRPC service name is required")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("gRPC call timeout for service %q must be greater than zero", service)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if conn, ok := f.conns[service]; ok {
		return conn, nil
	}

	// Traefik routes gRPC requests by authority; keep one endpoint while
	// selecting the Docker service router with the requested service name.
	conn, err := grpc.NewClient(
		f.target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(resilience.UnaryClientInterceptor(resilience.Config{
			Timeout: timeout,
			Retry: resilience.RetryConfig{
				MaxAttempts: 2,
				BaseDelay:   50 * time.Millisecond,
			},
		})),
		grpc.WithAuthority(service),
	)
	if err != nil {
		return nil, fmt.Errorf("create gRPC client for service %q via Traefik: %w", service, err)
	}
	f.conns[service] = conn
	return conn, nil
}

func (f *ClientFactory) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var firstErr error
	for service, conn := range f.conns {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close gRPC connection for service %q: %w", service, err)
		}
	}
	f.conns = make(map[string]*grpc.ClientConn)
	return firstErr
}
