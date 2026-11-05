package clients

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	freshnessv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/freshness/v1"
)

type fakeFreshness struct {
	freshnessv1.FreshnessServiceClient
	resp *freshnessv1.ApplyDecayResponse
	err  error
	req  *freshnessv1.ApplyDecayRequest
}

func (f *fakeFreshness) ApplyDecay(_ context.Context, in *freshnessv1.ApplyDecayRequest, _ ...grpc.CallOption) (*freshnessv1.ApplyDecayResponse, error) {
	f.req = in
	return f.resp, f.err
}

func TestApplyDecaySendsAsOfAndReturnsUpdatedCount(t *testing.T) {
	fake := &fakeFreshness{resp: &freshnessv1.ApplyDecayResponse{UpdatedCount: 7}}
	asOf := time.Date(2026, 11, 5, 7, 0, 0, 0, time.UTC)

	got, err := NewFreshnessClient(fake).ApplyDecay(context.Background(), asOf)
	if err != nil {
		t.Fatalf("ApplyDecay() error = %v", err)
	}
	if got != 7 {
		t.Errorf("ApplyDecay() = %d, want 7", got)
	}
	if fake.req == nil || !fake.req.GetAsOf().AsTime().Equal(asOf) {
		t.Errorf("request as_of = %v, want %v", fake.req.GetAsOf().AsTime(), asOf)
	}
}

func TestApplyDecayKeepsTheGRPCStatus(t *testing.T) {
	fake := &fakeFreshness{err: status.Error(codes.Unavailable, "freshness is down")}

	_, err := NewFreshnessClient(fake).ApplyDecay(context.Background(), time.Now())
	if status.Code(err) != codes.Unavailable {
		t.Errorf("status code = %v, want Unavailable (err = %v)", status.Code(err), err)
	}
}
