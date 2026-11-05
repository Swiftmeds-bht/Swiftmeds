package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	watchlistv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/watchlist/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/app"
	"github.com/Swiftmeds-bht/Swiftmeds/services/watchlist/internal/domain"
)

var now = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

type nearEverything struct{}

func (nearEverything) WithinRadius(context.Context, string, domain.Point, int32) (bool, error) {
	return true, nil
}

func newHandler() (*Handler, *app.Service) {
	svc := app.NewService(memory.NewRepository(), nearEverything{}, func() time.Time { return now }, func() string { return "sub-1" })
	return NewHandler(svc), svc
}

var thimphu = &commonv1.GeoPoint{Latitude: 27.4728, Longitude: 89.639}

func TestSubscribeListUnsubscribeRoundTrip(t *testing.T) {
	h, _ := newHandler()
	ctx := context.Background()

	sub, err := h.Subscribe(ctx, &watchlistv1.SubscribeRequest{UserId: "user-1", ProductId: "pr-1", Location: thimphu, RadiusM: 3000})
	if err != nil {
		t.Fatal(err)
	}
	got := sub.GetSubscription()
	if got.GetSubscriptionId() != "sub-1" || got.GetRadiusM() != 3000 || got.GetLocation().GetLatitude() != 27.4728 ||
		!got.GetCreatedAt().AsTime().Equal(now) || got.GetNotifiedAt() != nil {
		t.Errorf("Subscribe = %+v", got)
	}

	list, err := h.ListSubscriptions(ctx, &watchlistv1.ListSubscriptionsRequest{UserId: "user-1"})
	if err != nil || len(list.GetSubscriptions()) != 1 {
		t.Fatalf("ListSubscriptions = %v, %v", list, err)
	}

	if _, err := h.Unsubscribe(ctx, &watchlistv1.UnsubscribeRequest{SubscriptionId: "sub-1", UserId: "user-1"}); err != nil {
		t.Fatal(err)
	}
	list, _ = h.ListSubscriptions(ctx, &watchlistv1.ListSubscriptionsRequest{UserId: "user-1"})
	if len(list.GetSubscriptions()) != 0 {
		t.Errorf("after unsubscribe: %v", list.GetSubscriptions())
	}
}

func TestNotifiedSubscriptionHasNotifiedAt(t *testing.T) {
	h, svc := newHandler()
	ctx := context.Background()
	if _, err := h.Subscribe(ctx, &watchlistv1.SubscribeRequest{UserId: "user-1", ProductId: "pr-1", Location: thimphu}); err != nil {
		t.Fatal(err)
	}
	at := now.Add(time.Hour)
	if _, err := svc.HandleStockIncreased(ctx, domain.StockIncrease{PharmacyID: "ph-1", ProductID: "pr-1", NewAvailable: 2, OccurredAt: at}); err != nil {
		t.Fatal(err)
	}
	list, _ := h.ListSubscriptions(ctx, &watchlistv1.ListSubscriptionsRequest{UserId: "user-1"})
	if got := list.GetSubscriptions()[0].GetNotifiedAt(); got == nil || !got.AsTime().Equal(at) {
		t.Errorf("notified_at = %v, want %v", got, at)
	}
}

func TestErrorCodes(t *testing.T) {
	h, _ := newHandler()
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"subscribe without location", func() error {
			_, err := h.Subscribe(ctx, &watchlistv1.SubscribeRequest{UserId: "user-1", ProductId: "pr-1"})
			return err
		}, codes.InvalidArgument},
		{"subscribe with bad radius", func() error {
			_, err := h.Subscribe(ctx, &watchlistv1.SubscribeRequest{UserId: "user-1", ProductId: "pr-1", Location: thimphu, RadiusM: -5})
			return err
		}, codes.InvalidArgument},
		{"unsubscribe unknown", func() error {
			_, err := h.Unsubscribe(ctx, &watchlistv1.UnsubscribeRequest{SubscriptionId: "nope", UserId: "user-1"})
			return err
		}, codes.NotFound},
		{"list without user", func() error {
			_, err := h.ListSubscriptions(ctx, &watchlistv1.ListSubscriptionsRequest{})
			return err
		}, codes.InvalidArgument},
		{"stream alerts is not built yet", func() error {
			return h.StreamAlerts(&watchlistv1.StreamAlertsRequest{UserId: "user-1"}, nil)
		}, codes.Unimplemented},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Code(tt.call()); got != tt.want {
				t.Errorf("code = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnexpectedErrorsAreInternal(t *testing.T) {
	err := toStatus(errors.New("database is down"))
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
		t.Errorf("toStatus = %v", err)
	}
}
