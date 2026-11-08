package grpc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	expiryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/expiry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/expiry/internal/domain"
)

var t0 = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

// fakeHolds expires every hold it is asked about, unless told to fail.
type fakeHolds struct {
	err error
}

func (f fakeHolds) ExpireHold(context.Context, string) (domain.HoldStatus, error) {
	if f.err != nil {
		return "", f.err
	}
	return domain.HoldExpired, nil
}

func newHandler(t *testing.T, holds fakeHolds, deadlines map[string]time.Time) *Handler {
	t.Helper()
	repo := memory.NewRepository()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := app.NewService(repo, holds, app.Config{HoldDuration: 90 * time.Minute}, func() time.Time { return t0 }, log)
	for id, at := range deadlines {
		if err := svc.RecordDeadline(context.Background(), id, at); err != nil {
			t.Fatal(err)
		}
	}
	return NewHandler(svc)
}

func TestGetHoldDurationAnswersTheConfiguredDuration(t *testing.T) {
	h := newHandler(t, fakeHolds{}, nil)

	res, err := h.GetHoldDuration(context.Background(), &expiryv1.GetHoldDurationRequest{PharmacyId: "ph-1", ProductId: "pr-1"})

	if err != nil {
		t.Fatal(err)
	}
	if got := res.GetDuration().AsDuration(); got != 90*time.Minute {
		t.Errorf("duration = %v, want 1h30m", got)
	}
}

func TestRunSweepReturnsTheExpiredHoldsAndIsIdempotent(t *testing.T) {
	h := newHandler(t, fakeHolds{}, map[string]time.Time{
		"h-1": t0.Add(-time.Minute),
		"h-2": t0,
		"h-3": t0.Add(time.Minute),
	})
	req := &expiryv1.RunSweepRequest{AsOf: timestamppb.New(t0)}

	first, err := h.RunSweep(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.RunSweep(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"h-1", "h-2"}; !slices.Equal(first.GetExpiredHoldIds(), want) {
		t.Errorf("first sweep expired %v, want %v (h-2 expires exactly now)", first.GetExpiredHoldIds(), want)
	}
	if len(second.GetExpiredHoldIds()) != 0 {
		t.Errorf("second sweep expired %v, want nothing", second.GetExpiredHoldIds())
	}
}

func TestRunSweepWithoutAsOfUsesNow(t *testing.T) {
	h := newHandler(t, fakeHolds{}, map[string]time.Time{"h-1": t0})

	res, err := h.RunSweep(context.Background(), &expiryv1.RunSweepRequest{})

	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"h-1"}; !slices.Equal(res.GetExpiredHoldIds(), want) {
		t.Errorf("expired = %v, want %v", res.GetExpiredHoldIds(), want)
	}
}

func TestRunSweepFailsWhenHoldsIsDownAndNothingWasExpired(t *testing.T) {
	h := newHandler(t, fakeHolds{err: domain.ErrHoldsUnavailable}, map[string]time.Time{"h-1": t0})

	_, err := h.RunSweep(context.Background(), &expiryv1.RunSweepRequest{AsOf: timestamppb.New(t0)})

	if got := status.Code(err); got != codes.Unavailable {
		t.Fatalf("code = %s (err %v), want Unavailable", got, err)
	}
}

func TestRunSweepKeepsPartialProgress(t *testing.T) {
	// The first hold is expired, then Holds goes down: the answer is still OK with the first hold.
	repo := memory.NewRepository()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	holds := &flakyHolds{failFrom: 2}
	svc := app.NewService(repo, holds, app.Config{}, func() time.Time { return t0 }, log)
	for i, id := range []string{"h-1", "h-2"} {
		if err := svc.RecordDeadline(context.Background(), id, t0.Add(-time.Duration(2-i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	res, err := NewHandler(svc).RunSweep(context.Background(), &expiryv1.RunSweepRequest{AsOf: timestamppb.New(t0)})

	if err != nil {
		t.Fatalf("err = %v, want the partial result with no error", err)
	}
	if want := []string{"h-1"}; !slices.Equal(res.GetExpiredHoldIds(), want) {
		t.Errorf("expired = %v, want %v", res.GetExpiredHoldIds(), want)
	}
}

// flakyHolds expires the first holds it is asked about, then reports itself down.
type flakyHolds struct {
	failFrom int
	calls    int
}

func (f *flakyHolds) ExpireHold(context.Context, string) (domain.HoldStatus, error) {
	f.calls++
	if f.calls >= f.failFrom {
		return "", domain.ErrHoldsUnavailable
	}
	return domain.HoldExpired, nil
}

func TestToStatus(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{domain.ErrHoldsUnavailable, codes.Unavailable},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
		{context.Canceled, codes.Canceled},
		{errors.New("pq: password authentication failed"), codes.Internal},
	}
	for _, c := range cases {
		if got := status.Code(toStatus(c.err)); got != c.want {
			t.Errorf("toStatus(%v) = %s, want %s", c.err, got, c.want)
		}
	}
	if msg := status.Convert(toStatus(errors.New("pq: password authentication failed"))).Message(); msg != "internal error" {
		t.Errorf("message = %q, want a plain \"internal error\" that leaks nothing", msg)
	}
}
