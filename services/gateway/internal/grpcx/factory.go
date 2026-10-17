package grpcx

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ClientFactory struct {
	target string
	opts   []grpc.DialOption
	mu     sync.Mutex
	conns  map[string]*grpc.ClientConn
}

func NewClientFactory(traefikTarget string) *ClientFactory {
	return &ClientFactory{
		target: traefikTarget,
		opts: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		},
		conns: make(map[string]*grpc.ClientConn),
	}
}

// Connection returns a connection through Traefik. service is the Traefik service
// name, for example "auth" or "catalog".
func (f *ClientFactory) Connection(ctx context.Context, service string) (*grpc.ClientConn, error) {
	if service == "" {
		return nil, fmt.Errorf("gRPC service name is required")
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if conn, ok := f.conns[service]; ok {
		return conn, nil
	}

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Traefik routes gRPC requests by authority; keep one endpoint while
	// selecting the Docker service router with the requested service name.
	options := append([]grpc.DialOption{}, f.opts...)
	options = append(options, grpc.WithAuthority(service), grpc.WithBlock())
	conn, err := grpc.DialContext(dialCtx, f.target, options...)
	if err != nil {
		return nil, fmt.Errorf("dial Traefik for service %q: %w", service, err)
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
