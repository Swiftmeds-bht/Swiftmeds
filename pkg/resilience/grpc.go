package resilience

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config holds the settings for UnaryClientInterceptor. Zero fields use the defaults.
type Config struct {
	Breaker BreakerConfig
	Retry   RetryConfig
	// RetryableMethod decides whether a method may be retried after a
	// retryable failure. Zero allows only the read-only RPCs listed in
	// methods.go; writes such as CreateHold and unknown methods are called once.
	RetryableMethod func(method string) bool
	// Timeout is the limit for each attempt, not for the whole call. Default 3s.
	Timeout time.Duration
	// MaxConcurrent is how many calls may run at once. Default 100.
	MaxConcurrent int
}

// UnaryClientInterceptor applies the bulkhead, retry, circuit breaker and
// timeout to every unary call, in that order from the outside in:
//
//	bulkhead -> retry -> breaker -> timeout -> the call
//
// So one call uses one bulkhead slot however often it is retried, every attempt
// counts towards the breaker, and every attempt has its own timeout.
//
// By default only codes.Unavailable is retried. Other codes, including
// DeadlineExceeded, are not, because the server may already have done the work
// and a retry could do it twice. Change Config.Retry.Retryable for calls that are safe to repeat.
// Only read-only methods are retried at all (see methods.go and Config.RetryableMethod).
//
// Create one interceptor per connection so each downstream service gets its own breaker.
func UnaryClientInterceptor(cfg Config) grpc.UnaryClientInterceptor {
	if cfg.Breaker.IsFailure == nil {
		cfg.Breaker.IsFailure = isServerFault
	}
	if cfg.Retry.Retryable == nil {
		cfg.Retry.Retryable = isRetryable
	}
	if cfg.RetryableMethod == nil {
		cfg.RetryableMethod = isIdempotentMethod
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 100
	}
	breaker := NewBreaker(cfg.Breaker)
	bulkhead := NewBulkhead(cfg.MaxConcurrent)

	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := bulkhead.Execute(func() error {
			call := func(ctx context.Context) error {
				return breaker.Execute(func() error {
					return WithTimeout(ctx, cfg.Timeout, func(ctx context.Context) error {
						return invoker(ctx, method, req, reply, cc, opts...)
					})
				})
			}
			if !cfg.RetryableMethod(method) {
				return call(ctx)
			}
			return Retry(ctx, cfg.Retry, call)
		})
		return toStatus(err)
	}
}

// isRetryable allows a retry only for codes.Unavailable.
func isRetryable(err error) bool {
	return status.Code(err) == codes.Unavailable
}

// isServerFault counts only problems on the server side or the network.
// Bad input, NotFound and similar answers mean the service is working.
func isServerFault(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.Unknown, codes.DataLoss:
		return true
	default:
		return false
	}
}

// toStatus turns this package's errors into gRPC status errors for the caller.
func toStatus(err error) error {
	switch {
	case errors.Is(err, ErrOpen):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, ErrBulkheadFull):
		return status.Error(codes.ResourceExhausted, err.Error())
	}
	return err
}
