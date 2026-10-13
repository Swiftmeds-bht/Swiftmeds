// Package grpcx sets up a gRPC server the same way for every service:
// logging and panic-recovery interceptors, the standard gRPC health service,
// server reflection (for grpcurl), and graceful shutdown.
//
//	srv := grpcx.NewServer(log)
//	pricingv1.RegisterPricingServiceServer(srv, handler)
//	err := srv.Serve(ctx, ":50063") // returns after ctx is cancelled and calls have drained
package grpcx

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// DefaultShutdownTimeout is how long Serve waits for in-flight calls before forcing a stop.
const DefaultShutdownTimeout = 10 * time.Second

// Server wraps a grpc.Server with health reporting and graceful shutdown.
// It implements grpc.ServiceRegistrar, so generated Register*Server functions accept it.
type Server struct {
	grpc   *grpc.Server
	health *health.Server
	log    *slog.Logger

	// ShutdownTimeout overrides DefaultShutdownTimeout when > 0.
	ShutdownTimeout time.Duration
}

// NewServer returns a server with the standard interceptors, health service and reflection.
// Extra options are applied after the defaults.
func NewServer(log *slog.Logger, opts ...grpc.ServerOption) *Server {
	// The first interceptor is the outermost, so logging sees the Internal
	// status that recovery turns a panic into.
	base := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(LoggingUnary(log), RecoveryUnary(log)),
		grpc.ChainStreamInterceptor(LoggingStream(log), RecoveryStream(log)),
	}
	gs := grpc.NewServer(append(base, opts...)...)

	hs := health.NewServer()
	// NOT_SERVING until Serve is listening.
	hs.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	healthpb.RegisterHealthServer(gs, hs)
	reflection.Register(gs)

	return &Server{grpc: gs, health: hs, log: log}
}

// RegisterService implements grpc.ServiceRegistrar.
func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl any) {
	s.grpc.RegisterService(desc, impl)
}

// Health gives access to per-service health status, e.g. to report NOT_SERVING
// while the database is unreachable.
func (s *Server) Health() *health.Server {
	return s.health
}

// Serve listens on addr and blocks until ctx is cancelled or the server fails.
func (s *Server) Serve(ctx context.Context, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	return s.ServeListener(ctx, lis)
}

// ServeListener is Serve on an existing listener. Used in tests with bufconn.
//
// When ctx is cancelled it marks the server NOT_SERVING, stops accepting new
// calls, waits up to ShutdownTimeout for running calls to finish, then forces a stop.
// It returns nil after a clean shutdown.
func (s *Server) ServeListener(ctx context.Context, lis net.Listener) error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.grpc.Serve(lis) }()

	s.health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	s.log.Info("grpc server listening", slog.String("addr", lis.Addr().String()))

	select {
	case err := <-errCh:
		return fmt.Errorf("grpc serve: %w", err)
	case <-ctx.Done():
	}

	s.log.Info("grpc server shutting down")
	s.health.Shutdown()

	timeout := s.ShutdownTimeout
	if timeout <= 0 {
		timeout = DefaultShutdownTimeout
	}
	stopped := make(chan struct{})
	go func() {
		s.grpc.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(timeout):
		s.log.Warn("graceful shutdown timed out, forcing stop", slog.Duration("timeout", timeout))
		s.grpc.Stop()
		<-stopped
	}

	s.log.Info("grpc server stopped")
	return nil
}
