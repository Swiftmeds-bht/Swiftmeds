package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	watchlistv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
)

// fakeStream is the server side of a StreamAlerts call.
type fakeStream struct {
	grpc.ServerStreamingServer[watchlistv1.StockAlert]
	ctx  context.Context
	sent chan *watchlistv1.StockAlert
	err  error
}

func (f *fakeStream) Context() context.Context { return f.ctx }

func (f *fakeStream) Send(a *watchlistv1.StockAlert) error {
	if f.err != nil {
		return f.err
	}
	f.sent <- a
	return nil
}

func TestStreamAlertsSendsAlertsUntilTheClientCancels(t *testing.T) {
	h, svc := newHandler()
	ctx := context.Background()
	if _, err := h.Subscribe(ctx, &watchlistv1.SubscribeRequest{UserId: "user-1", ProductId: "pr-1", Location: thimphu}); err != nil {
		t.Fatal(err)
	}
	at := now.Add(time.Hour)
	if _, err := svc.HandleStockIncreased(ctx, domain.StockIncrease{PharmacyID: "ph-1", ProductID: "pr-1", NewAvailable: 7, OccurredAt: at}); err != nil {
		t.Fatal(err)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	s := &fakeStream{ctx: streamCtx, sent: make(chan *watchlistv1.StockAlert, 4)}
	done := make(chan error, 1)
	go func() { done <- h.StreamAlerts(&watchlistv1.StreamAlertsRequest{UserId: "user-1"}, s) }()

	select {
	case a := <-s.sent:
		if a.GetSubscriptionId() != "sub-1" || a.GetProductId() != "pr-1" || a.GetPharmacyId() != "ph-1" ||
			a.GetQuantityAvailable() != 7 || !a.GetOccurredAt().AsTime().Equal(at) {
			t.Errorf("alert = %+v", a)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no alert was streamed")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StreamAlerts after cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StreamAlerts did not end after the client cancelled")
	}
}

func TestStreamAlertsSendFailureIsInternal(t *testing.T) {
	h, svc := newHandler()
	ctx := context.Background()
	if _, err := h.Subscribe(ctx, &watchlistv1.SubscribeRequest{UserId: "user-1", ProductId: "pr-1", Location: thimphu}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.HandleStockIncreased(ctx, domain.StockIncrease{PharmacyID: "ph-1", ProductID: "pr-1", NewAvailable: 1, OccurredAt: now}); err != nil {
		t.Fatal(err)
	}
	err := h.StreamAlerts(&watchlistv1.StreamAlertsRequest{UserId: "user-1"}, &fakeStream{ctx: ctx, err: errors.New("broken pipe")})
	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal", status.Code(err))
	}
}
