package resilience

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeInvoker stands in for the real gRPC call. results[i] is the error for call i;
// calls after the end of the list succeed.
type fakeInvoker struct {
	results []error
	calls   int
}

func (f *fakeInvoker) invoke(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
	i := f.calls
	f.calls++
	if i < len(f.results) {
		return f.results[i]
	}
	return nil
}

func call(t *testing.T, in grpc.UnaryClientInterceptor, invoker grpc.UnaryInvoker) error {
	t.Helper()
	return in(context.Background(), "/svc/Method", nil, nil, nil, invoker)
}

func unavailable() error { return status.Error(codes.Unavailable, "down") }

// fastRetry makes retries instant.
func fastRetry(attempts int) RetryConfig {
	return RetryConfig{MaxAttempts: attempts, Sleep: (&recorder{}).Sleep}
}

func TestInterceptorRetriesUnavailableUntilItWorks(t *testing.T) {
	f := &fakeInvoker{results: []error{unavailable(), unavailable()}}
	in := UnaryClientInterceptor(Config{Retry: fastRetry(3)})

	if err := call(t, in, f.invoke); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if f.calls != 3 {
		t.Fatalf("calls = %d, want 3", f.calls)
	}
}

func TestInterceptorStopsAfterMaxAttempts(t *testing.T) {
	f := &fakeInvoker{results: []error{unavailable(), unavailable(), unavailable(), unavailable()}}
	in := UnaryClientInterceptor(Config{Retry: fastRetry(3)})

	err := call(t, in, f.invoke)

	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v, want Unavailable", status.Code(err))
	}
	if f.calls != 3 {
		t.Fatalf("calls = %d, want 3", f.calls)
	}
}

func TestInterceptorNeverRetriesNonRetryableCodes(t *testing.T) {
	for _, code := range []codes.Code{
		codes.InvalidArgument, codes.NotFound, codes.AlreadyExists, codes.PermissionDenied,
		codes.Unauthenticated, codes.FailedPrecondition, codes.Internal, codes.DeadlineExceeded,
		codes.ResourceExhausted, codes.Unknown, codes.Canceled,
	} {
		t.Run(code.String(), func(t *testing.T) {
			f := &fakeInvoker{results: []error{status.Error(code, "x"), nil}}
			in := UnaryClientInterceptor(Config{Retry: fastRetry(5)})

			err := call(t, in, f.invoke)

			if f.calls != 1 {
				t.Fatalf("calls = %d, want 1", f.calls)
			}
			if status.Code(err) != code {
				t.Fatalf("code = %v, want %v", status.Code(err), code)
			}
		})
	}
}

func TestInterceptorOpensTheBreakerAndFailsFast(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	f := &fakeInvoker{results: []error{unavailable(), unavailable()}}
	in := UnaryClientInterceptor(Config{
		Breaker: BreakerConfig{FailureThreshold: 2, Cooldown: testCooldown, Now: clock.Now},
		Retry:   fastRetry(1),
	})

	_ = call(t, in, f.invoke)
	_ = call(t, in, f.invoke)
	before := f.calls

	err := call(t, in, f.invoke)
	if status.Code(err) != codes.Unavailable || !strings.Contains(err.Error(), "circuit breaker") {
		t.Fatalf("err = %v, want Unavailable from the open breaker", err)
	}
	if f.calls != before {
		t.Fatal("the call reached the service while the breaker was open")
	}

	// After the cooldown one trial call goes through and closes the breaker.
	clock.Advance(testCooldown)
	if err := call(t, in, f.invoke); err != nil {
		t.Fatalf("trial call err = %v, want nil", err)
	}
	if err := call(t, in, f.invoke); err != nil {
		t.Fatalf("call after closing err = %v, want nil", err)
	}
}

func TestInterceptorDoesNotRetryWhenTheBreakerOpensDuringRetries(t *testing.T) {
	f := &fakeInvoker{results: []error{unavailable(), unavailable(), unavailable(), unavailable()}}
	in := UnaryClientInterceptor(Config{
		Breaker: BreakerConfig{FailureThreshold: 2},
		Retry:   fastRetry(5),
	})

	err := call(t, in, f.invoke)

	if f.calls != 2 {
		t.Fatalf("calls = %d, want 2 (the breaker opened after 2 failures)", f.calls)
	}
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v, want Unavailable", status.Code(err))
	}
}

func TestInterceptorBadRequestsDoNotOpenTheBreaker(t *testing.T) {
	notFound := status.Error(codes.NotFound, "no such medicine")
	f := &fakeInvoker{results: []error{notFound, notFound, notFound, notFound}}
	in := UnaryClientInterceptor(Config{
		Breaker: BreakerConfig{FailureThreshold: 2},
		Retry:   fastRetry(1),
	})

	for range 4 {
		_ = call(t, in, f.invoke)
	}

	if err := call(t, in, f.invoke); err != nil {
		t.Fatalf("err = %v, want nil: NotFound must not open the breaker", err)
	}
}

func TestInterceptorGivesEachAttemptItsOwnTimeout(t *testing.T) {
	slow := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		<-ctx.Done()
		return status.FromContextError(ctx.Err()).Err()
	}
	in := UnaryClientInterceptor(Config{Timeout: 20 * time.Millisecond, Retry: fastRetry(1)})

	err := call(t, in, slow)

	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("code = %v, want DeadlineExceeded", status.Code(err))
	}
}

func TestInterceptorBulkheadReturnsResourceExhausted(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	blocking := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		close(started)
		<-release
		return nil
	}
	in := UnaryClientInterceptor(Config{MaxConcurrent: 1})

	done := make(chan error, 1)
	go func() { done <- call(t, in, blocking) }()
	<-started

	f := &fakeInvoker{}
	err := call(t, in, f.invoke)

	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("code = %v, want ResourceExhausted", status.Code(err))
	}
	if f.calls != 0 {
		t.Fatal("the call reached the service while the bulkhead was full")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first call err = %v, want nil", err)
	}
}

func TestIsServerFault(t *testing.T) {
	cases := map[codes.Code]bool{
		codes.Unavailable: true, codes.DeadlineExceeded: true, codes.Internal: true,
		codes.Unknown: true, codes.DataLoss: true,
		codes.InvalidArgument: false, codes.NotFound: false, codes.PermissionDenied: false,
		codes.FailedPrecondition: false, codes.Canceled: false, codes.ResourceExhausted: false,
	}
	for code, want := range cases {
		if got := isServerFault(status.Error(code, "x")); got != want {
			t.Errorf("isServerFault(%v) = %v, want %v", code, got, want)
		}
	}
}
