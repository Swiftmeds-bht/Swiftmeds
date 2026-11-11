package clients

import (
	"context"
	"slices"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	expiryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/expiry/v1"
)

type fakeExpiry struct {
	expiryv1.ExpiryServiceClient
	resp *expiryv1.RunSweepResponse
	err  error
	req  *expiryv1.RunSweepRequest
}

func (f *fakeExpiry) RunSweep(_ context.Context, in *expiryv1.RunSweepRequest, _ ...grpc.CallOption) (*expiryv1.RunSweepResponse, error) {
	f.req = in
	return f.resp, f.err
}

func TestRunSweepSendsAsOfAndReturnsTheExpiredHolds(t *testing.T) {
	fake := &fakeExpiry{resp: &expiryv1.RunSweepResponse{ExpiredHoldIds: []string{"h-1", "h-2"}}}
	asOf := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

	got, err := NewExpiryClient(fake).RunSweep(context.Background(), asOf)

	if err != nil {
		t.Fatalf("RunSweep() error = %v", err)
	}
	if want := []string{"h-1", "h-2"}; !slices.Equal(got, want) {
		t.Errorf("RunSweep() = %v, want %v", got, want)
	}
	if fake.req == nil || !fake.req.GetAsOf().AsTime().Equal(asOf) {
		t.Errorf("request as_of = %v, want %v", fake.req.GetAsOf().AsTime(), asOf)
	}
}

func TestRunSweepReturnsAnEmptyListNotNil(t *testing.T) {
	fake := &fakeExpiry{resp: &expiryv1.RunSweepResponse{}}

	got, err := NewExpiryClient(fake).RunSweep(context.Background(), time.Now())

	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("got (%v, %v), want an empty non-nil list and no error", got, err)
	}
}

func TestRunSweepReturnsTheGRPCErrorAsItIs(t *testing.T) {
	fake := &fakeExpiry{err: status.Error(codes.Unavailable, "expiry down")}

	got, err := NewExpiryClient(fake).RunSweep(context.Background(), time.Now())

	if status.Code(err) != codes.Unavailable || got != nil {
		t.Errorf("got (%v, %v), want (nil, Unavailable)", got, err)
	}
}
