// Package resilience protects calls to other services from slow or failing
// dependencies. It has four small pieces that work alone or together:
//
//   - Breaker: stops calling a service that keeps failing (closed, open, half-open).
//   - Retry: tries again with exponential backoff and jitter.
//   - WithTimeout: gives each call a deadline.
//   - Bulkhead: limits how many calls run at the same time.
//
// For gRPC clients, one option adds all four:
//
//	conn, err := grpc.NewClient(addr,
//		grpc.WithTransportCredentials(insecure.NewCredentials()),
//		grpc.WithChainUnaryInterceptor(resilience.UnaryClientInterceptor(resilience.Config{})),
//	)
//
// Create one interceptor per connection, so each downstream service gets its
// own breaker and bulkhead.
package resilience
