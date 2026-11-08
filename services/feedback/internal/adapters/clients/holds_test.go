package clients

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	holdsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/holds/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/feedback/internal/domain"
)

type fakeHoldsClient struct {
	holdsv1.HoldsServiceClient
	resp *holdsv1.GetHoldResponse
	err  error
	req  *holdsv1.GetHoldRequest
}

func (f *fakeHoldsClient) GetHold(_ context.Context, in *holdsv1.GetHoldRequest, _ ...grpc.CallOption) (*holdsv1.GetHoldResponse, error) {
	f.req = in
	return f.resp, f.err
}

func holdWith(s holdsv1.HoldStatus) *holdsv1.GetHoldResponse {
	return &holdsv1.GetHoldResponse{Hold: &holdsv1.Hold{
		HoldId: "h-1", UserId: "u-1", PharmacyId: "ph-1", ProductId: "med-1", Status: s,
	}}
}

func TestGetHoldMapsTheHold(t *testing.T) {
	fake := &fakeHoldsClient{resp: holdWith(holdsv1.HoldStatus_HOLD_STATUS_COLLECTED)}

	got, err := Holds{C: fake}.GetHold(context.Background(), "h-1")
	if err != nil {
		t.Fatal(err)
	}

	want := domain.Hold{ID: "h-1", UserID: "u-1", PharmacyID: "ph-1", ProductID: "med-1", Collected: true}
	if got != want {
		t.Errorf("hold = %+v, want %+v", got, want)
	}
	if fake.req.GetHoldId() != "h-1" {
		t.Errorf("asked Holds for %q, want h-1", fake.req.GetHoldId())
	}
}

// Only COLLECTED counts. Every other state, including one added later, is not collected.
func TestOnlyACollectedHoldIsCollected(t *testing.T) {
	notCollected := []holdsv1.HoldStatus{
		holdsv1.HoldStatus_HOLD_STATUS_UNSPECIFIED,
		holdsv1.HoldStatus_HOLD_STATUS_PENDING,
		holdsv1.HoldStatus_HOLD_STATUS_ACTIVE,
		holdsv1.HoldStatus_HOLD_STATUS_EXPIRED,
		holdsv1.HoldStatus_HOLD_STATUS_CANCELLED,
		holdsv1.HoldStatus_HOLD_STATUS_REJECTED,
		holdsv1.HoldStatus(99),
	}
	for _, s := range notCollected {
		t.Run(s.String(), func(t *testing.T) {
			got, err := Holds{C: &fakeHoldsClient{resp: holdWith(s)}}.GetHold(context.Background(), "h-1")
			if err != nil {
				t.Fatal(err)
			}
			if got.Collected {
				t.Errorf("status %v counted as collected", s)
			}
		})
	}
}

func TestGetHoldMapsErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"not found", status.Error(codes.NotFound, "hold not found"), domain.ErrHoldNotFound},
		{"unavailable", status.Error(codes.Unavailable, "connection refused"), domain.ErrHoldsUnavailable},
		{"too slow", status.Error(codes.DeadlineExceeded, "deadline"), domain.ErrHoldsUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Holds{C: &fakeHoldsClient{err: tc.err}}.GetHold(context.Background(), "h-1")
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestGetHoldKeepsOtherErrorsAsTheyAre(t *testing.T) {
	_, err := Holds{C: &fakeHoldsClient{err: status.Error(codes.Internal, "boom")}}.GetHold(context.Background(), "h-1")

	if err == nil || errors.Is(err, domain.ErrHoldNotFound) || errors.Is(err, domain.ErrHoldsUnavailable) {
		t.Errorf("err = %v, want a plain error that is neither not-found nor unavailable", err)
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("status code = %v, want Internal kept", status.Code(err))
	}
}

func TestAnEmptyHoldIsNotFound(t *testing.T) {
	for _, resp := range []*holdsv1.GetHoldResponse{{}, {Hold: &holdsv1.Hold{}}} {
		_, err := Holds{C: &fakeHoldsClient{resp: resp}}.GetHold(context.Background(), "h-1")
		if !errors.Is(err, domain.ErrHoldNotFound) {
			t.Errorf("err = %v, want ErrHoldNotFound", err)
		}
	}
}
