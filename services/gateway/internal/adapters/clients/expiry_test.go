package clients

import (
	"context"
	"reflect"
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

func TestRunSweepSendsAsOfAndReturnsTheExpiredIDs(t *testing.T) {
	want := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
	fake := &fakeExpiry{resp: &expiryv1.RunSweepResponse{ExpiredHoldIds: want}}
	asOf := time.Date(2026, 11, 5, 7, 0, 0, 0, time.UTC)

	got, err := NewExpiryClient(fake).RunSweep(context.Background(), asOf)
	if err != nil {
		t.Fatalf("RunSweep() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RunSweep() = %v, want %v", got, want)
	}
	if fake.req == nil || !fake.req.GetAsOf().AsTime().Equal(asOf) {
		t.Errorf("request as_of = %v, want %v", fake.req.GetAsOf().AsTime(), asOf)
	}
}

func TestRunSweepKeepsTheGRPCStatus(t *testing.T) {
	fake := &fakeExpiry{err: status.Error(codes.Unavailable, "expiry is down")}

	_, err := NewExpiryClient(fake).RunSweep(context.Background(), time.Now())
	if status.Code(err) != codes.Unavailable {
		t.Errorf("status code = %v, want Unavailable (err = %v)", status.Code(err), err)
	}
}
