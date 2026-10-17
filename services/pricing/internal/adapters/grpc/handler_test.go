package grpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/common/v1"
	pricingv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pricing/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/adapters/memory"
	"github.com/Swiftmeds-bht/Swiftmeds/services/pricing/internal/app"
)

func newHandler() *Handler {
	now := func() time.Time { return time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC) }
	return NewHandler(app.NewService(memory.NewRepository(), now))
}

func TestSetThenGetRoundTrip(t *testing.T) {
	h := newHandler()
	ctx := context.Background()

	_, err := h.SetPrices(ctx, &pricingv1.SetPricesRequest{
		PharmacyId: "ph-1",
		Updates:    []*pricingv1.PriceUpdate{{ProductId: "pr-1", PriceChhertum: 4500}},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := h.BatchGetPrices(ctx, &pricingv1.BatchGetPricesRequest{
		Keys: []*commonv1.StockKey{
			{PharmacyId: "ph-1", ProductId: "pr-1"},
			{PharmacyId: "ph-1", ProductId: "unknown"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetPrices()) != 1 {
		t.Fatalf("got %d prices, want 1 (unknown key skipped)", len(resp.GetPrices()))
	}
	if got := resp.GetPrices()[0].GetPriceChhertum(); got != 4500 {
		t.Errorf("price = %d, want 4500", got)
	}
}

func TestSetPricesInvalidInputIsInvalidArgument(t *testing.T) {
	_, err := newHandler().SetPrices(context.Background(), &pricingv1.SetPricesRequest{
		PharmacyId: "ph-1",
		Updates:    []*pricingv1.PriceUpdate{{ProductId: "pr-1", PriceChhertum: 0}},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}
