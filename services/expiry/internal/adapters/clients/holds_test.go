package clients

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
)

type fakeHoldsClient struct {
	holdsv1.HoldsServiceClient
	resp *holdsv1.ExpireHoldResponse
	err  error
	req  *holdsv1.ExpireHoldRequest
}

func (f *fakeHoldsClient) ExpireHold(_ context.Context, in *holdsv1.ExpireHoldRequest, _ ...grpc.CallOption) (*holdsv1.ExpireHoldResponse, error) {
	f.req = in
	return f.resp, f.err
}

func TestExpireHoldSendsTheHoldIDAndReturnsTheStatus(t *testing.T) {
	cases := []struct {
		proto holdsv1.HoldStatus
		want  domain.HoldStatus
	}{
		{holdsv1.HoldStatus_HOLD_STATUS_PENDING, domain.HoldPending},
		{holdsv1.HoldStatus_HOLD_STATUS_ACTIVE, domain.HoldActive},
		{holdsv1.HoldStatus_HOLD_STATUS_COLLECTED, domain.HoldCollected},
		{holdsv1.HoldStatus_HOLD_STATUS_EXPIRED, domain.HoldExpired},
		{holdsv1.HoldStatus_HOLD_STATUS_CANCELLED, domain.HoldCancelled},
		{holdsv1.HoldStatus_HOLD_STATUS_REJECTED, domain.HoldRejected},
		{holdsv1.HoldStatus_HOLD_STATUS_UNSPECIFIED, ""},
	}
	for _, c := range cases {
		fake := &fakeHoldsClient{resp: &holdsv1.ExpireHoldResponse{Hold: &holdsv1.Hold{Status: c.proto}}}
		client := &HoldsClient{client: fake}

		got, err := client.ExpireHold(context.Background(), "h-1")

		if err != nil || got != c.want {
			t.Errorf("%s: got (%q, %v), want (%q, nil)", c.proto, got, err, c.want)
		}
		if fake.req.GetHoldId() != "h-1" {
			t.Errorf("request hold_id = %q, want h-1", fake.req.GetHoldId())
		}
	}
}

func TestExpireHoldMapsHoldsErrorsToDomainErrors(t *testing.T) {
	cases := []struct {
		code codes.Code
		want error
	}{
		{codes.NotFound, domain.ErrHoldNotFound},
		{codes.FailedPrecondition, domain.ErrNotYetExpired},
		{codes.Unavailable, domain.ErrHoldsUnavailable},
		{codes.DeadlineExceeded, domain.ErrHoldsUnavailable},
		{codes.Canceled, domain.ErrHoldsUnavailable},
	}
	for _, c := range cases {
		client := &HoldsClient{client: &fakeHoldsClient{err: status.Error(c.code, "from holds")}}

		_, err := client.ExpireHold(context.Background(), "h-1")

		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.code, err, c.want)
		}
	}
}

func TestExpireHoldKeepsOtherErrorsAsTheyAre(t *testing.T) {
	client := &HoldsClient{client: &fakeHoldsClient{err: status.Error(codes.Internal, "boom")}}

	_, err := client.ExpireHold(context.Background(), "h-1")

	if err == nil || errors.Is(err, domain.ErrHoldsUnavailable) || status.Code(errors.Unwrap(err)) != codes.Internal {
		t.Errorf("err = %v, want an unwrapped Internal error that does not stop the sweep", err)
	}
}

func TestExpireHoldFailsWhenHoldsReturnsNoHold(t *testing.T) {
	client := &HoldsClient{client: &fakeHoldsClient{resp: &holdsv1.ExpireHoldResponse{}}}

	if _, err := client.ExpireHold(context.Background(), "h-1"); err == nil {
		t.Fatal("err = nil, want an error")
	}
}
